// Step 1 · hello-loop —— 最小闭环：REPL 读入 → 裸 HTTP 调 Messages API → 打印回复
// 架构位置：L0.2(REPL 最小版) + L2.1(请求构造最小切片)
// 刻意不做：多轮 history / system / 重试 / streaming / thinking 展示 / 工具
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// ===== L2.1：与官方 Messages API 对齐的契约类型（本步只用到字段子集） =====

const (
	apiURL = "http://127.0.0.1:3425/v1/messages" // 本地 Magpie，Anthropic 兼容
	model  = "auto/glm-5.3"
)

// Message 一条对话消息。Step 1 的 content 只是纯字符串；
// Step 4 回填 tool_result 时它会变成块数组——届时再演进这个类型。
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Response 响应契约。content 是块数组：thinking | text | tool_use（本步只关心 text）。
type Response struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// httpClient 至少要有超时：reasoning 模型出词慢，但也不能无限等。
// （错误重试是 Step 2 的课题，这里失败就直接报。）
var httpClient = &http.Client{Timeout: 120 * time.Second}

// ask 发起一次单轮请求，返回 text 块拼接结果 + 原始响应（供打印观测行）
func ask(userText string) (string, Response, error) {
	body, err := json.Marshal(map[string]any{
		"model":      model,
		"max_tokens": 4096,
		"messages":   []Message{{Role: "user", Content: userText}},
	})
	if err != nil {
		return "", Response{}, err
	}
	resp, err := httpClient.Post(apiURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", Response{}, err
	}
	defer resp.Body.Close()

	var r Response
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", r, err
	}
	var sb strings.Builder
	for _, b := range r.Content {
		if b.Type == "text" { // thinking 块丢弃（Step 3 随流式一起展示）
			sb.WriteString(b.Text)
		}
	}
	return sb.String(), r, nil
}

// ===== L0.2：REPL 主循环（最小版） =====

func main() {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 64*1024), 1024*1024) // 允许粘贴长文本
	fmt.Println("hello-loop · Step 1 —— 输入消息回车发送，exit 退出")
	for {
		fmt.Print("\n你> ")
		if !sc.Scan() { // EOF / Ctrl+Z
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
		reply, r, err := ask(input)
		if err != nil {
			fmt.Println("[请求失败]", err)
			continue
		}
		fmt.Printf("\n助手> %s\n", reply)
		fmt.Printf("[stop_reason=%s tokens: in=%d out=%d]\n",
			r.StopReason, r.Usage.InputTokens, r.Usage.OutputTokens)
	}
}
