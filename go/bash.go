// Step 7 · bash.go —— L4.3 Bash 工具：把命令交给 shell 子进程，回传输出 + exit code
// 架构位置：L4.3(执行类工具) + L4.4(结果截断)
// 设计要点：Bash 只登记进 toolRegistry（同 S5/S6），loop/model 无需知道 shell 细节
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	bashDefaultTimeout = 120 * time.Second // 【社区证据（cc-haha）：utils/timeouts.ts DEFAULT_TIMEOUT_MS=120_000】
	bashMaxTimeout     = 600 * time.Second // 【社区证据（cc-haha）：utils/timeouts.ts MAX_TIMEOUT_MS=600_000】
	bashMaxOutput      = 30_000            // 【社区证据（cc-haha）：utils/shell/outputLimits.ts BASH_MAX_OUTPUT_DEFAULT】
)

// bashCwd 模拟「工作目录在命令之间持续」：每次子进程结束即销毁，只有我们记住的路径能传给下一次。
// 真实 cc-haha 由 appState 持久化 cwd（含 cd 检测器/hook），这里做最小版：只追踪「纯 cd <path>」。
var bashCwd = func() string {
	wd, _ := os.Getwd()
	return wd
}()

func init() {
	toolRegistry["Bash"] = bashTool // 登记进 L4.1 注册表；model.go 经 toolSpecs() 自动带出
}

// bashTool L4.3 执行类工具本体。
var bashTool = Tool{
	Name:        "Bash",
	Description: "Executes a given command and returns its output. The working directory persists between commands. " +
		"stdout and stderr are merged, and the result includes the exit code. Output is truncated if it exceeds " +
		"30000 characters.",
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The command to execute",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Optional timeout in milliseconds. Default 120000, max 600000.",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Clear, concise description of what this command does. Optional.",
			},
		},
		"required": []string{"command"},
	},
	Execute: executeBash,
}

// executeBash 执行一条命令：解析超时 → 追踪 cd → 启子进程 → 汇总输出与 exit code。
// 非零 exit code 以 error 形式返回（loop 侧据此置 is_error）——对齐 cc-haha DEFAULT_SEMANTIC。
func executeBash(args map[string]any) (string, error) {
	command, _ := args["command"].(string)
	if strings.TrimSpace(command) == "" {
		return "", errors.New("command is required")
	}

	// 1) 超时解析：默认 120s，上限 600s（超出即容忍上限）
	timeout := bashDefaultTimeout
	if v, ok := args["timeout"].(float64); ok {
		if ms := time.Duration(v) * time.Millisecond; ms > 0 && ms <= bashMaxTimeout {
			timeout = ms
		}
	}

	// 2) cd 追踪：先看这条命令是否把「下一个命令的工作目录」改掉了
	applyCdTracking(command)

	// 3) 选 shell：Windows → cmd.exe /C；其余 → bash -c（平台二分，教学简化）
	shell, shellArgs := resolveShell(command)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, shell, shellArgs...)
	cmd.Dir = bashCwd // 工作目录持续的关键：每次从我们记住的位置出发
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out // stderr 合并进 stdout【社区证据（cc-haha）：BashTool.tsx "stderr is merged into stdout"】

	err := cmd.Run()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			// 命令起来了、以非零码退出 → 走 DEFAULT_SEMANTIC：非 0 即失败
			return "", fmt.Errorf("exit code %d\n%s", ee.ExitCode(), truncateOutput(out.String()))
		}
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("command timed out after %s", timeout)
		}
		return "", fmt.Errorf("failed to start command: %w", err)
	}
	return truncateOutput(out.String()), nil
}

// truncateOutput 超 30000 字符截断并加提示行（L4.4 结果截断，防爆上下文）。
func truncateOutput(s string) string {
	if len(s) <= bashMaxOutput {
		return s
	}
	return s[:bashMaxOutput] +
		fmt.Sprintf("\n[... output truncated at %d characters ...]", bashMaxOutput)
}

// resolveShell 平台二分选择 shell 与参数形态。
// 真实 cc-haha 把 BashTool 与 PowerShellTool 分立为两个工具【社区证据（cc-haha）tools/ 目录】；
// 教学简化：按 runtime.GOOS 选一个原生 shell，不建第二个工具。
func resolveShell(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/C", command}
	}
	return "bash", []string{"-c", command}
}

// applyCdTracking 最小 cd 检测器：只追踪「纯 cd <path>」形态（无组合操作符），
// 成功且是目录才更新 bashCwd。组合命令（cd x && y）由 shell 自己在子进程内处理，不追踪。
func applyCdTracking(command string) {
	trimmed := strings.TrimSpace(command)
	lower := strings.ToLower(trimmed)
	if lower != "cd" && !strings.HasPrefix(lower, "cd ") && !strings.HasPrefix(lower, "cd\t") {
		return
	}
	rest := strings.TrimSpace(trimmed[2:])
	if rest == "" {
		return // 裸 cd → home：教学不做
	}
	if strings.ContainsAny(rest, "&|;><") {
		return // 组合命令，放弃追踪（诚实简化）
	}
	rest = strings.Trim(rest, `"`+"'")
	// 展开 ~ / 相对路径
	if rest == "~" || strings.HasPrefix(rest, "~/") {
		home := homeDir()
		if home == "" {
			return
		}
		if rest == "~" {
			rest = home
		} else {
			rest = filepath.Join(home, rest[2:])
		}
	}
	if !filepath.IsAbs(rest) {
		rest = filepath.Join(bashCwd, rest)
	}
	if fi, err := os.Stat(rest); err == nil && fi.IsDir() {
		bashCwd = rest
	}
}

// homeDir 返回用户主目录（cd ~ 用）。
func homeDir() string {
	if runtime.GOOS == "windows" {
		return os.Getenv("USERPROFILE")
	}
	return os.Getenv("HOME")
}