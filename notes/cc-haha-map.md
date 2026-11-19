# cc-haha 结构考古图（Step 0 产出）

> **证据纪律**：本文件所有内容来自 `reference/cc-haha`（Aemiser/cc-haha，fork 自 NanmiCoder/cc-haha）。
> 它自称「Claude Code 泄露源码的社区修复版」——**真伪无法验证**，且含 33+ 社区修改 commit
> （原生模块被 `stubs/`、`runtime/`、Python bridge 替换）。因此：
> - 从这里读到的一切，最高只能标 `【社区证据（cc-haha）】`，不得写成「Claude Code 官方事实」；
> - 「真实 CC 怎么做」的仲裁者永远是官方文档（(C) 线）；
> - 版本号自述 `999.0.0-local`，声称泄露于 2026-03-31，落后官方 v2.1.328。

## 技术栈（package.json 实测）

Bun + TypeScript + React 19 / Ink 6（TUI）+ @commander-js/extra-typings（argv）
+ @anthropic-ai/sdk 0.80 + @modelcontextprotocol/sdk 1.29 + zod 4 + ajv
+ OpenTelemetry 全家桶 + execa / diff / marked / shell-quote / picomatch
+ @anthropic-ai/sandbox-runtime（沙箱）+ growthbook（特性开关）

## 目录 → L 层映射表（后续每步对照的锚点）

| L 层 | cc-haha 锚点 | 行级证据 / 备注 |
|---|---|---|
| L0 入口/TUI | `src/main.tsx`(4690 行)、`src/replLauncher.tsx`、`src/entrypoints/cli.tsx`、`src/ink/`(48)、`src/components/`(144)、`src/screens/`、`src/keybindings/`、`src/vim/`、`src/voice/` | Ink+React 渲染层 |
| L0.1 argv | `@commander-js/extra-typings` 依赖；`src/cli/print.ts` | print.ts 含 stop_reason（`-p` 非交互模式） |
| L0.3 斜杠命令 | `src/commands/`(102 项)、`src/commands.ts` | /compact /insights /commit 等均在此 |
| L1 配置/记忆 | `src/utils/config.ts`(getGlobalConfig)、`src/setup.ts`、`src/constants/`(21)、`src/memdir/`(8, loadMemoryPrompt)、`src/services/settingsSync/`、`src/migrations/`(11) | QueryEngine.ts 头部可见对 config/memdir 的 import |
| L2 模型调用 | `src/services/api/claude.ts`、`withRetry.ts`、`errors.ts`(categorizeRetryableAPIError)、`logging.ts`、`src/services/tokenEstimation.ts` | stop_reason 落点；L2.3 重试在此 |
| L2.4 成本 | `src/cost-tracker.ts`、`src/costHook.ts` | accumulateUsage / getTotalCost |
| L3 Agent Loop | `src/query.ts`(1729 行) + `src/QueryEngine.ts`(1295 行) | 双双命中 stop_reason + tool_result；QueryEngine 是编排层，query 是内层循环（待逐行验证）；`utils/abortController` → L3.4 中断 |
| L4 工具注册表 | `src/Tool.ts`(792 行，接口：zod v4 + ToolUseBlockParam/ToolResultBlockParam)、`src/tools.ts`(389 行) | tools/ 共 44 个工具目录 |
| L4.2 文件工具 | `tools/FileReadTool` `FileWriteTool` `FileEditTool` `GlobTool` `GrepTool` `NotebookEditTool` `LSPTool` | 与官方「内置工具五分类」对得上【官方事实：how-claude-code-works】 |
| L4.3 执行 | `tools/BashTool` `PowerShellTool`（平台分立）、`tools/TaskCreate/Get/List/Output/Stop/Update*`（后台任务） | |
| L5 权限 | `src/hooks/toolPermission/`(PermissionContext、coordinatorHandler)、`src/hooks/useCanUseTool.ts`(CanUseToolFn)、`PermissionMode` 类型、`entrypoints/sandboxTypes.ts`、`@anthropic-ai/sandbox-runtime` 依赖 | `EnterPlanModeTool/ExitPlanModeTool` 在 tools/ |
| L6 会话/压缩 | `src/history.ts`(464)、`src/services/compact/compact.ts` + `sessionMemoryCompact.ts`、`utils/conversationRecovery.ts`、`bootstrap/state.ts`、`utils/fileHistory.ts`(checkpoint 快照，被 QueryEngine import) | compact 与官方 auto-compact 描述对应【官方事实】 |
| L7.1 MCP | `src/services/mcp/`(types.ts: MCPServerConnection)、`tools/MCPTool` `ToolSearchTool`(延迟加载) `ListMcpResourcesTool` `ReadMcpResourceTool` `McpAuthTool` | ToolSearchTool ↔ 官方 tool search【官方事实】 |
| L7.2 Hooks | 疑似 `src/utils/hooks/hookHelpers.ts`(registerStructuredOutputEnforcement 被 QueryEngine import)；⚠️ `src/hooks/`(85 项) 大部分是 React UI hooks，**CC 产品意义上的 Hooks 引擎位置待验证** | Step 12 精读时定位 |
| L7.3 Subagent | `src/tools/AgentTool/`(loadAgentsDir)、`src/Task.ts`、`src/tasks/`(9)、`tools/SendMessageTool`、`src/coordinator/` | |
| L7.4 其他 | `src/skills/`(4) + `tools/SkillTool`、`src/plugins/`(2)、`tools/TodoWriteTool`、`tools/ScheduleCronTool`、`tools/WorkflowTool`、`TeamCreate/DeleteTool` | 本课程选学边界 |
| L8 可观测 | OTel 依赖 + `services/internalLogging.ts`、`services/analytics/`、`services/diagnosticTracking.ts`、`services/api/logging.ts` | OTel 存在性从「未验证」升级为【社区证据（cc-haha）】 |
| 未识别/外围 | `bridge/`(31, claudeInChrome)、`remote/`、`upstreamproxy/`、`native-ts/`、`buddy/`、`assistant/`、`moreright/`、`vendor/`、`outputStyles/` | 外围能力，主线不涉及，遇到再考 |

## 每步精读队列（Step → 首要文件）

S1→`entrypoints/cli.tsx` 头部；S2→`services/api/claude.ts`+`withRetry.ts`；S3→SSE 处理(api/ 内)；
S4→`query.ts` 主循环+`Tool.ts` 接口；S5/S6→`tools/FileReadTool/`、`FileEditTool/`；
S7→`tools/BashTool/`；S8→`hooks/toolPermission/`；S9→`services/compact/compact.ts`+`history.ts`；
S10→`utils/config.ts`+`memdir/`；S11→`services/mcp/`；S12→定位 Hooks 引擎；S13→`tools/AgentTool/`；S14→OTel 接线。
