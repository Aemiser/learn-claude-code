// Step 5 · tools.go —— L4 工具层：注册表 + 内置工具（get_time / Read / Write）
// 架构位置：L4.1(Tool 接口与注册表) + L4.2a(文件读写) + L4.4(行号与截断后处理)
// 本步设计要点：新增两个工具只登记进 toolRegistry，loop.go / model.go 一行不改
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	// ===== L4.2b · Edit：唯一匹配替换（diff 式修改） =====
	"Edit": {
		Name: "Edit",
		Description: "Performs exact string replacement in a file. old_string must match the file content " +
			"exactly, including whitespace and indentation, and must be unique in the file " +
			"(include surrounding lines for context if needed). Fails if old_string is not found or matches more than once " +
			"unless replace_all is true. You must Read the file before editing it. " +
			"Prefer this tool over Write for modifications — it only sends the diff.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type": "string", "description": "Absolute path to the file to modify",
				},
				"old_string": map[string]any{
					"type": "string", "description": "Exact text to replace (must be unique in the file)",
				},
				"new_string": map[string]any{
					"type": "string", "description": "Replacement text",
				},
				"replace_all": map[string]any{
					"type": "boolean", "description": "Replace all occurrences. Optional, default false.",
				},
			},
			"required": []string{"file_path", "old_string", "new_string"},
		},
		Execute: func(args map[string]any) (string, error) {
			fp, _ := args["file_path"].(string)
			oldS, _ := args["old_string"].(string)
			newS, _ := args["new_string"].(string)
			replaceAll, _ := args["replace_all"].(bool)
			if fp == "" || oldS == "" && newS == "" {
				return "", errors.New("file_path, old_string and new_string are required")
			}
			if !filepath.IsAbs(fp) {
				return "", fmt.Errorf("file_path must be absolute, got: %s", fp)
			}
			if oldS == newS {
				// 【社区证据（cc-haha）：FileEditTool.ts:148】No changes to make
				return "", errors.New("no changes to make: old_string and new_string are exactly the same")
			}
			// read-first 闸：改文件比写文件更需要知道现状
			if !readFirst[fp] {
				if _, err := os.Stat(fp); err == nil {
					return "", fmt.Errorf("file %s exists but was not Read in this session; Read it first before editing", fp)
				}
				if oldS != "" {
					return "", fmt.Errorf("file %s does not exist; use Write to create it", fp)
				}
			}
			data, err := os.ReadFile(fp)
			if err != nil {
				if os.IsNotExist(err) && oldS == "" {
					// 空 old + 不存在 → 创建新文件【社区证据（cc-haha）：FileEditTool.ts:225】
					if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
						return "", err
					}
					if err := os.WriteFile(fp, []byte(newS), 0o644); err != nil {
						return "", err
					}
					readFirst[fp] = true
					return fmt.Sprintf("created %s (%d bytes)", fp, len(newS)), nil
				}
				return "", err
			}
			content := string(data)
			// 匹配计数三分支【社区证据（cc-haha）：FileEditTool.ts:316-336】
			matches := strings.Count(content, oldS)
			switch {
			case matches == 0:
				return "", fmt.Errorf("string to replace not found in file.\nString: %s", oldS)
			case matches > 1 && !replaceAll:
				return "", fmt.Errorf(
					"found %d matches of the string to replace, but replace_all is false. "+
						"To replace all occurrences, set replace_all to true. "+
						"To replace only one occurrence, include more surrounding lines to make old_string unique.\nString: %s",
					matches, oldS)
			}
			replaced := strings.Replace(content, oldS, newS, 1)
			if replaceAll {
				replaced = strings.ReplaceAll(content, oldS, newS)
			}
			if err := os.WriteFile(fp, []byte(replaced), 0o644); err != nil {
				return "", err
			}
			// 成功回传改动处 ±3 行片段（带行号），供模型自查落点
			return editSnippet(replaced, newS), nil
		},
	},

	// ===== L4.2b · Glob：按模式找文件名 =====
	"Glob": {
		Name:        "Glob",
		Description: "Fast file pattern matching. Supports glob patterns like \"**/*.go\" (** matches any depth). Returns matching file paths sorted, up to 100 results.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type": "string", "description": "Glob pattern, e.g. **/*.go or src/**/*.ts",
				},
				"path": map[string]any{
					"type": "string", "description": "Directory to search in. Optional, defaults to current directory.",
				},
			},
			"required": []string{"pattern"},
		},
		Execute: func(args map[string]any) (string, error) {
			pattern, _ := args["pattern"].(string)
			root, _ := args["path"].(string)
			if pattern == "" {
				return "", errors.New("pattern is required")
			}
			if root == "" {
				root = "."
			}
			absRoot, err := filepath.Abs(root)
			if err != nil {
				return "", err
			}
			re, err := globToRegexp(pattern)
			if err != nil {
				return "", err
			}
			var matches []string
			err = filepath.WalkDir(absRoot, func(p string, d os.DirEntry, err error) error {
				if err != nil {
					return nil // 不可读的目录跳过，不让整个搜索崩掉
				}
				if d.IsDir() {
					return nil
				}
				rel, rerr := filepath.Rel(absRoot, p)
				if rerr != nil {
					return nil
				}
				rel = filepath.ToSlash(rel)
				if re.MatchString(rel) {
					matches = append(matches, p)
					if len(matches) >= 100 {
						return filepath.SkipAll // 限量 100，防爆结果
					}
				}
				return nil
			})
			if err != nil {
				return "", err
			}
			if len(matches) == 0 {
				return "no files matched", nil
			}
			sort.Strings(matches)
			return strings.Join(matches, "\n") + fmt.Sprintf("\n[%d files]", len(matches)), nil
		},
	},

	// ===== L4.2b · Grep：按正则搜内容 =====
	"Grep": {
		Name:        "Grep",
		Description: "Searches file contents with a regular expression. Returns matches in \"file:line:content\" format, up to 100 results. Searches recursively under the given path (default: current directory).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type": "string", "description": "Regular expression to search for",
				},
				"path": map[string]any{
					"type": "string", "description": "File or directory to search. Optional, defaults to current directory.",
				},
			},
			"required": []string{"pattern"},
		},
		Execute: func(args map[string]any) (string, error) {
			pattern, _ := args["pattern"].(string)
			root, _ := args["path"].(string)
			if pattern == "" {
				return "", errors.New("pattern is required")
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return "", fmt.Errorf("invalid regex: %v", err)
			}
			if root == "" {
				root = "."
			}
			info, err := os.Stat(root)
			if err != nil {
				return "", err
			}
			var hits []string
			scanFile := func(p string) error {
				data, err := os.ReadFile(p)
				if err != nil {
					return nil // 二进制/无权限跳过
				}
				for i, line := range strings.Split(string(data), "\n") {
					if re.MatchString(line) {
						hits = append(hits, fmt.Sprintf("%s:%d:%s", p, i+1, strings.TrimSpace(line)))
						if len(hits) >= 100 {
							return errLimitReached // 限量哨兵
						}
					}
				}
				return nil
			}
			if !info.IsDir() {
				err = scanFile(filepath.ToSlash(root))
			} else {
				err = filepath.WalkDir(root, func(p string, d os.DirEntry, werr error) error {
					if werr != nil || d.IsDir() {
						return nil
					}
					return scanFile(p)
				})
			}
			if err != nil && err != errLimitReached {
				return "", err
			}
			if len(hits) == 0 {
				return "no matches", nil
			}
			return strings.Join(hits, "\n") + fmt.Sprintf("\n[%d matches]", len(hits)), nil
		},
	},
}

// errLimitReached Grep/Glob 限量哨兵（内部用）
var errLimitReached = errors.New("result limit reached")

// editSnippet 返回 newS 首次出现处 ±3 行的带行号片段（供模型自查改动落点）
func editSnippet(content, newS string) string {
	lines := strings.Split(content, "\n")
	joined := strings.Join(lines, "\n")
	idx := strings.Index(joined, newS)
	if idx < 0 {
		return "edit applied"
	}
	// 由字符偏移算行号
	line := strings.Count(joined[:idx], "\n")
	start, end := line-3, line+3
	if start < 0 {
		start = 0
	}
	if end > len(lines)-1 {
		end = len(lines) - 1
	}
	var sb strings.Builder
	for i := start; i <= end; i++ {
		fmt.Fprintf(&sb, "%6d\t%s\n", i+1, lines[i])
	}
	return sb.String()
}

// globToRegexp 把 glob 模式编译为正则：** 跨目录、* 单段内、? 单字符（教学实现）
func globToRegexp(pattern string) (*regexp.Regexp, error) {
	var sb strings.Builder
	sb.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				// "**"：跨任意目录（吃掉紧随的 / 也可以）
				sb.WriteString("(?:.*/)?")
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++ // "**/" 已被 (?:.*/)? 覆盖
				}
			} else {
				sb.WriteString("[^/]*")
			}
		case '?':
			sb.WriteString("[^/]")
		case '.':
			sb.WriteString(`\.`)
		default:
			sb.WriteByte(c)
		}
	}
	sb.WriteString("$")
	return regexp.Compile(sb.String())
}
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
