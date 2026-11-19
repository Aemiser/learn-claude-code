// Step 3 · model.go —— L2 模型调用层：契约类型 + 流式请求 + 错误重试
// 架构位置：L2.1 请求构造(完整：system/messages/max_tokens/stream)
//           L2.2 SSE 解析(自 S2 迁入) / L2.3 重试(新增) / L2.4 usage 账目
// 层间契约：本文件不做任何 UI——流式事件经 StreamHooks 回调交给调用方渲染
// 刻意不做：tools 字段(S4) / baseURL、model 配置化(S10) / 上下文修剪(S9)
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

// ===== L2.1 契约类型 =====

// Message 一条对话消息（S4 回填 tool_result 时 Content 将演化为块数组）
type Message struct {
	Role    string `json:"role"` // "user" | "assistant"
	Content string `json:"content"`
}

// apiError 带 HTTP 状态码的错误——L2.3 重试判定的依据
type apiError struct {
	Status int
	Body   string
}

func (e *apiError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.Status, e.Body) }

// ChatResult 一轮流式请求的汇总：回复文本 + 方向盘(stop_reason) + 账目(usage)
type ChatResult struct {
	Text          string
	StopReason    string
	InputTokens   int
	OutputTokens  int
	streamStarted bool // 收到过任一增量即 true：中途断流不可安全重试
}

// StreamHooks L2→调用方 的渲染契约：模型层只报告发生了什么，怎么画是上层的事
type StreamHooks struct {
	OnBlockStart func(blockType string)       // thinking | text |（S4 起 tool_use）
	OnDelta      func(deltaType, text string) // thinking_delta | text_delta
	OnBlockStop  func(blockType string)
}

// ===== L2.2 SSE 事件契约（迁自 main.go，字段不变；序列见 notes/steps/step-02.md） =====

type deltaInfo struct {
	Type       string `json:"type"` // text_delta | thinking_delta | signature_delta
	Text       string `json:"text"`
	Thinking   string `json:"thinking"`
	StopReason string `json:"stop_reason"` // 仅 message_delta 携带
}

type streamEvent struct {
	Type         string    `json:"type"`
	Index        int       `json:"index"`
	Delta        deltaInfo `json:"delta"`
	ContentBlock struct {
		Type string `json:"type"`
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

// chatStream 发起一次流式请求；messages 为完整多轮历史
func chatStream(ctx context.Context, messages []Message, hooks StreamHooks) (ChatResult, error) {
	body, _ := json.Marshal(map[string]any{
		"model":      modelID,
		"max_tokens": maxTokens,
		"stream":     true,
		"system":     systemPrompt, // L2.1 补全：system 与 messages 分离
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
				res.Text += ev.Delta.Text
				if hooks.OnDelta != nil {
					hooks.OnDelta("text_delta", ev.Delta.Text)
				}
			}
		case "content_block_stop":
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
func chatWithRetry(ctx context.Context, messages []Message, hooks StreamHooks) (ChatResult, error) {
	const maxAttempts = 4
	backoff := 500 * time.Millisecond
	for attempt := 1; ; attempt++ {
		res, err := chatStream(ctx, messages, hooks)
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
