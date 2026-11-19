# 架构图（L0–L8 编号 · 随学习更新）

状态：✅已实装 ｜ 🟡桩/部分 ｜ ⬜未覆盖 ｜ (B)=cc-haha 对照锚点（见 notes/cc-haha-map.md）

```
┌──────────────────────────────────────────────────────────────┐
│ L0 入口与呈现 🟡S1     (B) main.tsx / ink/ / commands/      │
│   L0.1 argv与模式 ⬜    (B) commander / cli/print.ts         │
│   L0.2 REPL与渲染 🟡S1 (B) replLauncher.tsx                 │
│   L0.3 斜杠命令 ⬜      (B) commands/                        │
├──────────────────────────────────────────────────────────────┤
│ L1 配置与记忆 ⬜        (B) utils/config.ts / memdir/        │
│   L1.1 settings层级 ⬜  L1.2 env/密钥 ⬜  L1.3 记忆注入 ⬜    │
├──────────────────────────────────────────────────────────────┤
│ L3 Agent Loop ⬜        (B) query.ts + QueryEngine.ts        │
│   L3.1 骨架 ⬜ L3.2 解析分发 ⬜ L3.3 回填 ⬜                 │
│   L3.4 停止/中断 ⬜     L3.5 并发 ⬜                          │
│        ▲            ┌── L5 权限与安全 ⬜ (B) toolPermission/ │
│        └──► L4 工具注册表与内置工具 ⬜ (B) Tool.ts + tools/  │
├──────────────────────────────────────────────────────────────┤
│ L2 模型调用 🟡S3       (B) services/api/{claude,withRetry}  │
│   L2.1 请求构造 🟡S1 L2.2 streaming 🟡S2 L2.3 重试 🟡S3 L2.4 🟡S1│
├──────────────────────────────────────────────────────────────┤
│ L6 上下文与会话 🟡S3   (B) history.ts / services/compact/   │
│   L6.1 history 🟡S3 L6.2 compact ⬜ L6.3 持久化resume ⬜     │
├──────────────────────────────────────────────────────────────┤
│ L7 扩展机制 ⬜          (B) services/mcp/ + AgentTool/ 等    │
│   L7.1 MCP ⬜ L7.2 Hooks ⬜ L7.3 Subagent ⬜ L7.4 选学 ⬜     │
├──────────────────────────────────────────────────────────────┤
│ L8 可观测与评测 ⬜      (B) OTel + internalLogging           │
└──────────────────────────────────────────────────────────────┘
端点：本地 Magpie http://127.0.0.1:3425 /v1/messages（Anthropic 兼容，免鉴权）
模型：auto/glm-5.3（注意 thinking block）｜ 语言：Go（项目内 1.25 工具链，系统 1.22 不动）
(A)线实装：go/main.go(L0.2 REPL + L6.1 内存 history/回滚) + go/model.go(L2：SSE 解析、重试、
StreamHooks 契约、system/messages 完整请求) —— 里程碑 M1：流式多轮聊天机
```

## 数据流走查（一次「把 a.py 的 foo 改名为 bar」）

L0.2 收输入 → L0.3 非斜杠 → L1 装配 system prompt 与权限规则 → L3.1 组 messages →
L2.1/2.2 流式请求 → L3.2 解析 tool_use(Read) → L5 放行 → L4.2 执行+L4.4 截断加行号 →
L3.3 回填 tool_result → 再请求 → tool_use(Edit) → L5.2 确认 → 执行回填 →
循环至 stop_reason=end_turn → L0.2 渲染 → L6.1 追加 history（近上限则 L6.2 compact）→ L8 记账。

## 各层契约速记

- L2↔L3：`{system, messages, tools}` → response(content blocks[], stop_reason, usage)
- L3↔L4：`Tool = { name, description, inputSchema(JSON Schema), execute(args)→ToolResult }`
- L5↔L3：`decide(tool, args) → allow | deny | ask`
- content block 三态（Magpie/GLM 实测）：`thinking | text | tool_use`；回填用 `tool_result`（tool_use_id 配对，is_error 标错）——契约见官方 how-tool-use-works【官方事实】
