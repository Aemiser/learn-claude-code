// Step 4 · loop.go —— L3 Agent Loop：tool_use ↔ tool_result 循环
// 架构位置：L3.1 循环骨架 / L3.2 解析分发 / L3.3 回填 / L3.4 停止条件
// 这是 chatbot 与 agent 的分界线：模型不再只是"说完"，而是"要求做事、拿到结果、再说"
package main

import (
	"context"
	"fmt"
	"strings"
)

const maxRounds = 10 // L3.4 停止条件：防止模型死循环烧钱

// TurnSummary 一轮用户输入的汇总（L2.4 累计账目 + L3 轮数）
type TurnSummary struct {
	StopReason   string
	InputTokens  int // 累计（一次 turn 可能多次调用模型）
	OutputTokens int // 累计
	Rounds       int // 模型被调用的次数
}

// runLoop 处理一轮用户输入：反复「请求→解析 tool_use→执行→回填 tool_result」，
// 直到模型不再请求工具（或超限）。
// 入参 history 已含刚追加的 user 消息；返回更新后的 history（含全部中间消息）。
func runLoop(ctx context.Context, history []Message, hooks StreamHooks) ([]Message, TurnSummary, error) {
	var sum TurnSummary
	for {
		if ctx.Err() != nil {
			return history, sum, ctx.Err()
		}
		res, err := chatWithRetry(ctx, history, toolSpecs(), hooks)
		if err != nil {
			return history, sum, err
		}
		sum.Rounds++
		sum.InputTokens += res.InputTokens
		sum.OutputTokens += res.OutputTokens

		// 把 assistant 消息（含 text 与 tool_use 块）写入历史。thinking 已剔除。
		history = append(history, Message{Role: "assistant", Content: res.Blocks})

		if !res.toolUseSeen { // 无工具调用：正常说完，本轮结束
			sum.StopReason = res.StopReason
			return history, sum, nil
		}
		if sum.Rounds >= maxRounds { // L3.4 停止条件
			sum.StopReason = "max_rounds"
			return history, sum, fmt.Errorf("超过最大轮数 %d", maxRounds)
		}

		// L3.3 回填：执行每个 tool_use 块，生成 tool_result 块
		var results []ContentBlock
		for _, b := range res.Blocks {
			if b.Type != "tool_use" {
				continue
			}
			tool, ok := toolRegistry[b.Name]
			if !ok { // 模型可能编造不存在的工具名，回一个 is_error 让它知道
				results = append(results, ContentBlock{
					Type: "tool_result", ToolUseID: b.ID,
					Content: fmt.Sprintf("unknown tool: %s", b.Name), IsError: true,
				})
				continue
			}
			fmt.Printf("🔧 调用工具 %s(%s)\n", b.Name, compactArgs(b.Input))
			out, execErr := tool.Execute(b.Input)
			tr := ContentBlock{Type: "tool_result", ToolUseID: b.ID}
			if execErr != nil {
				tr.IsError = true
				tr.Content = execErr.Error()
				fmt.Printf("✗ 工具失败: %v\n", execErr)
			} else {
				tr.Content = out
				fmt.Printf("✓ 工具返回: %s\n", truncate(out, 120))
			}
			results = append(results, tr)
		}
		// tool_result 必须以 user 角色回传【官方事实：how-tool-use-works】
		history = append(history, Message{Role: "user", Content: results})
	}
}

// compactArgs 打印工具参数（教学可视化）
func compactArgs(args map[string]any) string {
	parts := []string{}
	for k, v := range args {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	return strings.Join(parts, ", ")
}

// truncate 只截终端显示，完整结果照常回传模型
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
