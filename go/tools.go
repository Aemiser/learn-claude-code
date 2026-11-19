// Step 4 · tools.go —— L4.1 工具注册表：统一抽象 + 规格导出
// 架构位置：L4.1(Tool 接口与注册表)
// 契约：模型只见 Name/Description/InputSchema（JSON Schema）；Execute 由 harness 本地调用
package main

import "time"

// Tool L4.1 的统一抽象。官方契约：工具 = 描述(让模型知道何时用) + schema(让模型知道怎么传参)
// + 执行器(本地真正做事)【官方事实：how-tool-use-works 的"三个执行位置"】
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any            // 手写 JSON Schema（Go 无 zod，反而更透）
	Execute     func(args map[string]any) (string, error)
}

// toolRegistry 内置工具表。S5 起 Read/Write 等文件工具都登记在这里。
// 用 map 字面量直接声明，不做 init() 魔法。
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
