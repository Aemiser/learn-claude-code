// Step 4 · model.go —— L2 模型调用层：契约类型 + 流式请求 + 错误重试
// 架构位置：L2.1 请求构造 / L2.2 SSE 解析（新增 tool_use 块）/ L2.3 重试 / L2.4 usage
// 层间契约：本文件不做 UI，流式事件经 StreamHooks 回调；工具规格由 tools 参数注入
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	apiURL       = "http://127.0.0.1:3425/v1/messages" // 本地 Magpie，Anthropic 兼容
	modelID      = "auto/glm-5.3"
	maxTokens    = 4096
	systemPrompt = "你是运行在终端里的编程助手。用用户使用的语言回答，简洁直接。"
)

// Message 一条对话消息。Content 二态：string（纯文本）或 []ContentBlock（含工具调用/结果）
type Message struct {
	Role    string `json:"role"` // "user" | "assistant"
	Content any    `json:"content"`
}

// ContentBlock 官方契约的内容块：text | tool_use | tool_result（thinking 不入历史）
type ContentBlock struct {
	Type      string         `json:"type"`                  // "text" | "tool_use" | "tool_result"
	Text      string         `json:"text,omitempty"`        // text 块
	ID        string         `json:"id,omitempty"`          // tool_use 块（工具调用唯一 id）
	Name      string         `json:"name,omitempty"`        // tool_use 块（工具名）
	Input     map[string]any `json:"input,omitempty"`       // tool_use 块（解析后的参数对象）
	ToolUseID string         `json:"tool_use_id,omitempty"` // tool_result 块（必须与 tool_use.id 配对）
	Content   any            `json:"content,omitempty"`     // tool_result 块
	IsError   bool           `json:"is_error,omitempty"`    // tool_result 块（执行失败标记）
}

// apiError 带 HTTP 状态码的错误——L2.3 重试判定的依据
type apiError struct {
	Status int
	Body   string
}

func (e *apiError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.Status, e.Body) }

// ChatResult 一轮流式请求的汇总：内容块 + 方向盘(stop_reason) + 账目(usage)
type ChatResult struct {
	Blocks        []ContentBlock // 本轮 assistant 消息的内容块（text + tool_use，thinking 剔除）
	Text          string         // 便捷：所有 text 块拼接
	StopReason    string
	InputTokens   int
	OutputTokens  int
	streamStarted bool // 收到过任一增量即 true：中途断流不可安全重试（S3-1）
	toolUseSeen   bool // 本轮出现过 tool_use 块——cc-haha 教训：stop_reason 不可靠，以此为准【社区证据（cc-haha）：query.ts:554】
}

// StreamHooks L2→调用方 的渲染契约：模型层只报告发生了什么，怎么画是上层的事
type StreamHooks struct {
	OnBlockStart func(blockType string)       // thinking | text | tool_use
	OnDelta      func(deltaType, text string) // thinking_delta | text_delta
	OnBlockStop  func(blockType string)
}

// ===== L2.2 SSE 事件契约 =====

type deltaInfo struct {
	Type        string `json:"type"` // text_delta | thinking_delta | input_json_delta
	Text        string `json:"text"`
	Thinking    string `json:"thinking"`
	PartialJSON string `json:"partial_json"` // input_json_delta 携带：工具参数的 JSON 片段
	StopReason  string `json:"stop_reason"`  // 仅 message_delta 携带
}

type streamEvent struct {
	Type  string    `json:"type"`
	Index int       `json:"index"`
	Delta deltaInfo `json:"delta"`
	ContentBlock struct {
		Type string `json:"type"` // thinking | text | tool_use
		ID   string `json:"id"`   // tool_use 块的 id
		Name string `json:"name"` // tool_use 块的工具名
	} `json:"content_block"`
	Message struct {
		Usage struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	} `json:"message"`
	Usage struct { // message_delta 携带真值（message_start 里是占位 0）
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

var httpClient = &http.Client{Timeout: 180 * time.Second}

// chatStream 发起一次流式请求。tools 为工具规格（L4 注入）；messages 为完整历史。
// 返回的 Blocks 是「可回填给模型」的内容块：text + tool_use，thinking 剔除。
func chatStream(ctx context.Context, messages []Message, tools []map[string]any, hooks StreamHooks) (ChatResult, error) {
	body, _ := json.Marshal(map[string]any{
		"model":      modelID,
		"max_tokens": maxTokens,
		"stream":     true,
		"system":     systemPrompt,
		"tools":      tools, // S4：tools 字段就位
		"messages":   messages,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return ChatResult{}, err
	}
	req.Header.Set("content-type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return ChatResult{}, err // 网络错误：是否重试由上层判定
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return ChatResult{}, &apiError{Status: resp.StatusCode, Body: string(b)}
	}

	var res ChatResult
	var blockType string
	var textBuf string                                 // 当前 text 块累积（防多 text 块互相污染）
	var toolUseID, toolUseName, toolInputJSON string  // 当前 tool_use 块累积态
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue // "event:" 行与空行跳过：信息都在 data 的 JSON 里
		}
		var ev streamEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "content_block_start":
			blockType = ev.ContentBlock.Type
			switch blockType {
			case "text":
				textBuf = ""
			case "tool_use":
				toolUseID = ev.ContentBlock.ID
				toolUseName = ev.ContentBlock.Name
				toolInputJSON = ""
				res.toolUseSeen = true // 判定信号：不信任 stop_reason
			}
			if hooks.OnBlockStart != nil {
				hooks.OnBlockStart(blockType)
			}
		case "content_block_delta":
			switch ev.Delta.Type {
			case "thinking_delta":
				res.streamStarted = true
				if hooks.OnDelta != nil {
					hooks.OnDelta("thinking_delta", ev.Delta.Thinking)
				}
			case "text_delta":
				res.streamStarted = true
				textBuf += ev.Delta.Text
				res.Text += ev.Delta.Text
				if hooks.OnDelta != nil {
					hooks.OnDelta("text_delta", ev.Delta.Text)
				}
			case "input_json_delta":
				res.streamStarted = true
				toolInputJSON += ev.Delta.PartialJSON // 工具参数是 JSON 片段，需拼接
			}
		case "content_block_stop":
			switch blockType {
			case "text":
				res.Blocks = append(res.Blocks, ContentBlock{Type: "text", Text: textBuf})
			case "tool_use":
				var input map[string]any
				json.Unmarshal([]byte(toolInputJSON), &input) // 片段拼完整后再解析
				res.Blocks = append(res.Blocks, ContentBlock{
					Type: "tool_use", ID: toolUseID, Name: toolUseName, Input: input,
				})
			}
			if hooks.OnBlockStop != nil {
				hooks.OnBlockStop(blockType)
			}
		case "message_delta": // 方向盘与账目以此事件为准
			res.StopReason = ev.Delta.StopReason
			res.InputTokens, res.OutputTokens = ev.Usage.InputTokens, ev.Usage.OutputTokens
		}
	}
	if err := sc.Err(); err != nil {
		if ctx.Err() == context.Canceled {
			return res, context.Canceled // Ctrl+C：上层负责回滚轮次
		}
		return res, err // 中途断流（streamStarted=true）：上层不会重试
	}
	return res, nil
}

// ===== L2.3 错误重试 =====

// retryable 网络错误默认可重试；apiError 仅 429/5xx（参数类 4xx 重试无意义）
func retryable(err error) bool {
	var ae *apiError
	if errors.As(err, &ae) {
		return ae.Status == 429 || ae.Status >= 500
	}
	return true
}

// chatWithRetry 指数退避重试：最多 4 次尝试，500ms × 2ⁿ。
// 三种情况立即放弃：用户取消 / 流已开始（重试会重复输出）/ 错误不可重试。
func chatWithRetry(ctx context.Context, messages []Message, tools []map[string]any, hooks StreamHooks) (ChatResult, error) {
	const maxAttempts = 4
	backoff := 500 * time.Millisecond
	for attempt := 1; ; attempt++ {
		res, err := chatStream(ctx, messages, tools, hooks)
		if err == nil {
			return res, nil
		}
		if errors.Is(err, context.Canceled) || res.streamStarted ||
			attempt >= maxAttempts || !retryable(err) {
			return res, err
		}
		fmt.Fprintf(os.Stderr, "[retry %d/%d] %v\n", attempt, maxAttempts-1, err)
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}
}
