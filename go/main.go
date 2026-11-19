// Step 3 · main.go —— L0.2 REPL + L6.1 雏形（history）；L2 已拆至 model.go
// 架构位置：L0.2(REPL 与渲染) + L6.1(内存 history，无修剪)
// 刻意不做：斜杠命令(S9) / argv(S10) / history 持久化(S9)
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
)

const (
	ansiDim   = "\x1b[2m"
	ansiReset = "\x1b[0m"
)

// consoleHooks 把 L2 的流式事件翻译成终端渲染——L2 不需要知道 ANSI 的存在
func consoleHooks() StreamHooks {
	return StreamHooks{
		OnBlockStart: func(blockType string) {
			if blockType == "thinking" {
				fmt.Print(ansiDim + "〔思考〕")
			}
		},
		OnDelta: func(deltaType, text string) {
			if deltaType == "thinking_delta" {
				fmt.Print(ansiDim + text)
			} else {
				fmt.Print(text)
			}
		},
		OnBlockStop: func(blockType string) {
			if blockType == "thinking" {
				fmt.Print(ansiReset + "\n〔回答〕")
			}
		},
	}
}

func main() {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	history := []Message{} // L6.1 雏形：本课程的第一个"状态"
	fmt.Println("multi-turn · Step 3 —— 多轮记忆 + model.go 模块化 + 重试；exit 退出")
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
		history = append(history, Message{Role: "user", Content: input})
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		res, err := chatWithRetry(ctx, history, consoleHooks())
		stop()
		if err != nil {
			// 回滚本轮 user 消息，保持 user/assistant 严格交替（S2-2 的答案）
			history = history[:len(history)-1]
			if errors.Is(err, context.Canceled) {
				fmt.Print(ansiReset + "\n[本轮已中断，未计入历史]\n")
			} else {
				fmt.Println("\n[请求失败]", err)
			}
			continue
		}
		history = append(history, Message{Role: "assistant", Content: res.Text})
		fmt.Printf("\n[stop_reason=%s tokens: in=%d out=%d | history=%d 条]\n",
			res.StopReason, res.InputTokens, res.OutputTokens, len(history))
	}
}
