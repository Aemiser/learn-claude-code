# 学习计划书（已锁定版 · Step 0 落盘）

目标：深度理解 terminal coding agent（以 Claude Code 为参照系）的分层架构，并亲手用 Go 造一个最小可用复刻：能读/改文件、跑命令、循环调用工具、带权限确认与上下文管理。

## 三条腿

- **(A) 主线项目**：本目录从零手写的 Go 复刻 agent，唯一被运行和改动的项目。
- **(B) 可读源码对照**：主对照 `reference/cc-haha`（自称泄露源码修复版，**只读/不运行/不复制代码**，一切结论标【社区证据（cc-haha）】）；辅对照 `anthropics/anthropic-sdk-go` 的 examples（Go 语言实践）。
- **(C) 事实基线**：官方文档与文章，唯一有权裁定「真实 CC 怎么做」。
  核心三篇：code.claude.com/docs/en/how-claude-code-works、platform.claude.com 的 how-tool-use-works、code.claude.com/docs/en/agent-sdk/ 系列。

每步固定顺序：(C) 定事实 → (B) 读对应锚点文件 → (A) 写最小实现 → 三者对照。

## 已锁定决策

| 决策 | 值 |
|---|---|
| 语言 | Go；**系统 Go 1.22.12 不动**，本项目 go.mod 声明 `go 1.25`，经 GOTOOLCHAIN=auto + goproxy.cn 自动取隔离工具链 |
| 模型端点 | 本地 Magpie `http://127.0.0.1:3425`，**Anthropic 兼容 `/v1/messages`**，免鉴权 |
| 默认模型 | `auto/glm-5.3`（路由组）；测试用 `*glm-5.3-flash*` |
| 注意 | GLM-5.3 是 reasoning 模型：content blocks 含 `thinking` 类型，L3.2 必须处理 thinking/text/tool_use 三种 |
| cc-haha 红线 | 只读不运行；不复制代码进 (A) 线；标签纪律 |

## 路线（Step 0 + 14 步）

| # | 标题 | 层 | 验收要点 |
|---|---|---|---|
| 0 | cc-haha 结构考古 | 全层索引 | notes/cc-haha-map.md（本步已完成） |
| 1 | hello-loop：最小问答 | L0.2+L2.1 | 单 main.go 裸 HTTP，中文问答 |
| 2 | streaming(SSE) | L2.2 | 逐字输出；Ctrl+C 中断本轮（原 S3，应用户要求调序） |
| 3 | 模型层模块化+多轮+重试 | L2.1/L2.3+L6.1 | 三轮记忆；断网重试（M1） |
| 4 | 第一次 tool loop | L3.1-3.3+L4.1 | get_time 全链路转起来 |
| 5 | Read+Write | L4.2a/L4.4 | 行号、截断 |
| 6 | Edit+Glob+Grep | L4.2b | 一条指令自动改文件（M2） |
| 7 | Bash+并发 | L4.3+L3.5 | 跑命令；多 tool_use 并行 |
| 8 | 权限层 | L5.1-5.3 | 危险命令 y/n；路径越界拦截 |
| 9 | 会话持久化+/compact | L6+L0.3 | resume 恢复；compact 降 token（M3） |
| 10 | 配置与记忆+argv | L1+L0.1 | settings 层级；CLAUDE.md 注入 |
| 11 | MCP client | L7.1 | 外部 server 工具被模型调用 |
| 12 | Hooks | L7.2 | PreToolUse 阻断 .env 写入 |
| 13 | Subagent | L7.3 | Task 委派，主上下文不膨胀 |
| 14 | 可观测+总复盘 | L8+L2.4 | --debug 回放全流程（M4） |

依赖：1→…→11 严格串行；12/13 互不依赖；14 收尾。里程碑 M1=S3、M2=S6、M3=S9、M4=S14。

## 教学简化边界（诚实清单）

TUI 用 bufio 起步（Bubble Tea 为选修）；权限只做 Manual+规则（Auto 档分类器是服务端能力，无法本地复刻）；sandbox 只讲不做；compact 单策略；MCP 默认手写 JSON-RPC/stdio、只做 tools；subagent 为进程内子 loop；hooks 只做 2 事件；Skills/Plugins/Checkpointing 不实现；evals 只读文章讨论。逐项详见各 Step 的【2】节。

## 跨会话约定

`PLAN.md`(本文件) / `ARCHITECTURE.md`(L0-L8 编号+状态) / `PROGRESS.md`(每步必更) /
`METHODOLOGY.md`(学习方法论·规则与想法持续收集，每步收尾检查、每3步归纳) / `notes/`(层笔记+步骤导读+考古图)。
换会话说「继续」，按 PROGRESS.md 接续。
