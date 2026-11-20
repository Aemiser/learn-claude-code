// Step 5 · tools.go —— L4 工具层：注册表 + 内置工具（get_time / Read / Write）
// 架构位置：L4.1(Tool 接口与注册表) + L4.2a(文件读写) + L4.4(行号与截断后处理)
// 本步设计要点：新增两个工具只登记进 toolRegistry，loop.go / model.go 一行不改
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// maxLinesToRead Read 默认最多读的行数。对齐 cc-haha 的 MAX_LINES_TO_READ=2000
// 【社区证据（cc-haha）：FileReadTool/prompt.ts:10】
const maxLinesToRead = 2000

// readFirst 会话级「读过哪些文件」记录：Write 覆盖已存在文件前查表（read-first 防护）。
// 对应 cc-haha 的 FileStateCache 思想【社区证据（cc-haha）】；教学简化为进程内 map。
var readFirst = map[string]bool{}

// Tool L4.1 的统一抽象。官方契约：工具 = 描述(让模型知道何时用) + schema(让模型知道怎么传参)
// + 执行器(本地真正做事)【官方事实：how-tool-use-works 的"三个执行位置"】
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any            // 手写 JSON Schema（Go 无 zod，反而更透）
	Execute     func(args map[string]any) (string, error)
}

// toolRegistry 内置工具表。工具名对齐真实 CC 的 PascalCase（Read/Write）
// 【社区证据（cc-haha）：FILE_READ_TOOL_NAME='Read'】
var toolRegistry = map[string]Tool{
	"get_time": {
		Name:        "get_time",
		Description: "Get the current local time in a timezone",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"timezone": map[string]any{
					"type":        "string",
					"description": "IANA timezone name, e.g. Asia/Shanghai; empty means local time",
				},
			},
		},
		Execute: func(args map[string]any) (string, error) {
			tz, _ := args["timezone"].(string)
			if tz == "" {
				tz = "Local"
			}
			loc, err := time.LoadLocation(tz)
			if err != nil {
				return "", err
			}
			return time.Now().In(loc).Format("2006-01-02 15:04:05 MST"), nil
		},
	},

	// ===== L4.2a · Read：读文件 → cat -n 行号 → 截断（L4.4） =====
	"Read": {
		Name: "Read",
		Description: "Reads a file from the local filesystem. file_path must be an absolute path. " +
			"Results are returned using cat -n format, with line numbers starting at 1. " +
			"By default it reads up to 2000 lines from the beginning of the file; " +
			"use offset and limit to read specific parts of longer files.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the file to read",
				},
				"offset": map[string]any{
					"type":        "integer",
					"description": "Line number to start reading from (1-based). Optional.",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of lines to read. Optional, default 2000.",
				},
			},
			"required": []string{"file_path"},
		},
		Execute: func(args map[string]any) (string, error) {
			fp, _ := args["file_path"].(string)
			if fp == "" {
				return "", errors.New("file_path is required")
			}
			if !filepath.IsAbs(fp) {
				return "", fmt.Errorf("file_path must be absolute, got: %s", fp)
			}
			data, err := os.ReadFile(fp)
			if err != nil {
				return "", err // 文件不存在等 → is_error 回填，模型自会应对
			}
			lines := strings.Split(string(data), "\n")
			total := len(lines)

			offset := 1 // 1-based，默认从头
			if v, ok := args["offset"].(float64); ok && int(v) >= 1 {
				offset = int(v)
			}
			limit := maxLinesToRead // 默认 2000，不允许超过
			if v, ok := args["limit"].(float64); ok && int(v) >= 1 && int(v) < limit {
				limit = int(v)
			}
			start := offset - 1
			if start >= total {
				return "", fmt.Errorf("offset %d is beyond end of file (%d lines)", offset, total)
			}
			end := start + limit
			if end > total {
				end = total
			}
			truncated := end < total // 还有没显示的行 → 截断提示（而非报错）

			var sb strings.Builder
			for i := start; i < end; i++ {
				fmt.Fprintf(&sb, "%6d\t%s\n", i+1, lines[i]) // cat -n 风格：行号右对齐6位+tab
			}
			if truncated {
				fmt.Fprintf(&sb, "[showing lines %d-%d of %d; use offset=%d to read more]",
					offset, end, total, end+1)
			}
			readFirst[fp] = true // 登记：Write 覆盖此文件前的 read-first 检查放行
			return sb.String(), nil
		},
	},

	// ===== L4.2a · Write：写文件（read-first 防护） =====
	"Write": {
		Name: "Write",
		Description: "Writes a file to the local filesystem, overwriting the existing file if one exists. " +
			"If the file already exists, you MUST Read it first or this call fails. " +
			"Prefer the Edit tool for small changes (it sends only the diff); use Write for new files or complete rewrites. " +
			"Parent directories are created automatically.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the file to write",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Full content to write to the file",
				},
			},
			"required": []string{"file_path", "content"},
		},
		Execute: func(args map[string]any) (string, error) {
			fp, _ := args["file_path"].(string)
			content, _ := args["content"].(string)
			if fp == "" {
				return "", errors.New("file_path is required")
			}
			if !filepath.IsAbs(fp) {
				return "", fmt.Errorf("file_path must be absolute, got: %s", fp)
			}
			// read-first 防护：覆盖已存在文件前，本会话必须 Read 过
			// 【社区证据（cc-haha）：FileWriteTool/prompt.ts "This tool will fail if you did not read the file first"】
			if _, err := os.Stat(fp); err == nil && !readFirst[fp] {
				return "", fmt.Errorf(
					"file %s exists but was not Read in this session; Read it first before overwriting", fp)
			}
			if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
				return "", err
			}
			if err := os.WriteFile(fp, []byte(content), 0o644); err != nil {
				return "", err
			}
			readFirst[fp] = true // 自己写的内容视为已知最新
			return fmt.Sprintf("wrote %d bytes to %s", len(content), fp), nil
		},
	},
}

// toolSpecs 把注册表转成 API 请求的 tools 字段。
// 注意：map 迭代顺序随机，但 tools 数组顺序对模型无意义，可接受。
func toolSpecs() []map[string]any {
	specs := []map[string]any{}
	for _, t := range toolRegistry {
		specs = append(specs, map[string]any{
			"name":         t.Name,
			"description":  t.Description,
			"input_schema": t.InputSchema,
		})
	}
	return specs
}
