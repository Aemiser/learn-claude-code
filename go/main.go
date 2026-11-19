// Step 2 · stream-loop —— SSE 流式 REPL：thinking(暗色) + text 逐 token 打印
// 架构位置：L0.2(REPL) + L2.2(streaming 最小实现)
// 刻意不做：多轮 history / 错误重试 / 拆 model.go —— 全部留给 Step 3
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"
)

const (
	apiURL = "http://127.0.0.1:3425/v1/messages" // 本地 Magpie，Anthropic 兼容
	model  = "auto/glm-5.3"
)

// Message 一条消息（沿用 Step 1；Step 3 会随 history 一起搬进 model.go）
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ===== L2.2：SSE 事件契约（只声明我们消费的字段；序列见 notes/steps/step-02.md） =====

// deltaInfo 三种载荷合一：text_delta 带 Text；thinking_delta 带 Thinking；
// message_delta 的 delta 带 StopReason。signature_delta 忽略。
type deltaInfo struct {
	Type       string `json:"type"`
	Text       string `json:"text"`
	Thinking   string `json:"thinking"`
	StopReason string `json:"stop_reason"`
}

type streamEvent struct {
	Type         string    `json:"type"`
	Index        int       `json:"index"`
	Delta        deltaInfo `json:"delta"`
	ContentBlock struct { // 仅 content_block_start 携带：thinking | text（S4 后还会有 tool_use）
		Type string `json:"type"`
	} `json:"content_block"`
	Message struct { // 仅 message_start 携带（input_tokens 是占位 0，真值在 message_delta）
		Usage struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	} `json:"message"`
	Usage struct { // message_delta 携带真值
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

var httpClient = &http.Client{Timeout: 180 * time.Second}

const (
	ansiDim   = "\x1b[2m"
	ansiReset = "\x1b[0m"
)

// chatStream 发起一次流式请求，边收边打印；返回 stop_reason 与 token 账目。
// ctx 取消（Ctrl+C）时中断本轮而不报错——这是 L3.4 中断机制的第一颗种子。
func chatStream(ctx context.Context, userText string) (string, int, int, error) {
	body, _ := json.Marshal(map[string]any{
		"model":      model,
		"max_tokens": 4096,
		"stream":     true, // ← 本步主角：流式开关
		"messages":   []Message{{Role: "user", Content: userText}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return "", 0, 0, err
	}
	req.Header.Set("content-type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", 0, 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}

	stopReason, inTok, outTok := "", 0, 0
	inThinking := false // 当前块是否为 thinking 块（决定打印样式）

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue // "event:" 行与空行都跳过：信息全在 data 的 JSON 里
		}
		var ev streamEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "content_block_start":
			inThinking = ev.ContentBlock.Type == "thinking"
			if inThinking {
				fmt.Print(ansiDim + "〔思考〕")
			}
		case "content_block_delta":
			switch ev.Delta.Type {
			case "thinking_delta":
				fmt.Print(ev.Delta.Thinking)
			case "text_delta":
				fmt.Print(ev.Delta.Text)
			}
		case "content_block_stop":
			if inThinking {
				fmt.Print(ansiReset + "\n〔回答〕")
				inThinking = false
			}
		case "message_delta": // 账目以此为准（message_start 里的 usage 是占位）
			stopReason = ev.Delta.StopReason
			inTok, outTok = ev.Usage.InputTokens, ev.Usage.OutputTokens
		}
	}
	if err := sc.Err(); err != nil {
		if ctx.Err() == context.Canceled {
			fmt.Print(ansiReset + "\n[本轮已中断]")
			return "interrupted", inTok, outTok, nil
		}
		return stopReason, inTok, outTok, err
	}
	return stopReason, inTok, outTok, nil
}

// ===== L0.2：REPL 主循环 =====

func main() {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	fmt.Println("stream-loop · Step 2 —— SSE 流式输出；回答中 Ctrl+C 中断本轮；exit 退出")
	for {
		fmt.Print("\n你> ")
		if !sc.Scan() {
			break
		}
		input := strings.TrimSpace(sc.Text())
		switch input {
		case "":
			continue
		case "exit", "quit":
			fmt.Println("bye")
			return
		}
		// 每轮一个可取消 context：Ctrl+C 只打断当前请求；提示符空闲时 Ctrl+C 仍是默认行为(退出进程)
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		stopReason, inTok, outTok, err := chatStream(ctx, input)
		stop()
		if err != nil {
			fmt.Println("\n[请求失败]", err)
			continue
		}
		fmt.Printf("\n[stop_reason=%s tokens: in=%d out=%d]\n", stopReason, inTok, outTok)
	}
}
