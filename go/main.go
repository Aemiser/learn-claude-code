// Step 4 · main.go —— L0.2 REPL + L6.1（history）；loop 已拆至 loop.go，L2 在 model.go
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
	fmt.Println("agent-loop · Step 4 —— 模型会调用工具了（get_time）；exit 退出")
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
		snap := len(history) // 回滚锚点：失败时撤销整个 turn（含所有工具中间消息）
		history = append(history, Message{Role: "user", Content: input})
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		history, sum, err := runLoop(ctx, history, consoleHooks())
		stop()
		if err != nil {
			history = history[:snap] // 回滚式状态管理：失败即还原，保持交替不变量
			if errors.Is(err, context.Canceled) {
				fmt.Print(ansiReset + "\n[本轮已中断，未计入历史]\n")
			} else {
				fmt.Println("\n[请求失败]", err)
			}
			continue
		}
		fmt.Printf("\n[stop_reason=%s tokens: in=%d out=%d | 轮数=%d | history=%d 条]\n",
			sum.StopReason, sum.InputTokens, sum.OutputTokens, sum.Rounds, len(history))
	}
}
