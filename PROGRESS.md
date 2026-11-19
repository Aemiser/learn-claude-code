# 进度（每步必更）

## 当前状态

- 阶段：Phase 2 进行中
- 已完成：Phase 0 校准、Phase 1 计划、Step 0（考古）、**Step 1（hello-loop）**
- 下一步：Step 2 模型层模块化 + 多轮 history + 错误重试（L2.1 完整/L2.3/L6.1 雏形）
- 阻塞项：无

## 决策日志

| 日期 | 决策 |
|---|---|
| 2026-11-19 | Phase 0 证伪「CC 开源」；三条腿模型确立 |
| 2026-11-19 | 引入 reference/cc-haha（B 线主对照），三条红线：只读/不复制/标签纪律 |
| 2026-11-19 | 技术栈 Go；系统 1.22 不动，项目内用 1.25 工具链（GOTOOLCHAIN=auto + goproxy.cn） |
| 2026-11-19 | 模型端点=本地 Magpie 127.0.0.1:3425 /v1/messages（Anthropic 兼容，免鉴权）；默认 auto/glm-5.3 |
| 2026-11-19 | 工作约定：代码在 go/ 下；每步先写 notes/steps/step-XX.md 前置导读；每步一个 commit |

## Step 记录

### Step 1 ✅（2026-11-19）
- 产出：go/go.mod（go 1.25 隔离工具链生效）、go/main.go（~105 行）、notes/steps/step-01.md
- 验证：管道实测「用一个词回答：你好」→ 回复「你好！」，stop_reason=end_turn，in=17 out=164
- 发现：GOTOOLCHAIN=auto 经 goproxy.cn 自动取 go1.25.0 成功；系统 1.22 未动
- git 教学：本步曾把 step1 文档更新误卷进 step0 提交，用 reset --soft + amend 重放修正——commit 要与步骤原子对应
- 遗留（后续步骤处理）：Ctrl+C 硬退出无 handler（S3）；thinking 块丢弃未展示（S3）；无重试（S2）；无多轮记忆（S2）

### Step 0 ✅（2026-11-19）
- 产出：notes/cc-haha-map.md（L0-L8 锚点映射+证据）、PLAN.md、ARCHITECTURE.md、PROGRESS.md、.gitignore；git init
- 关键发现：cc-haha 的 query.ts(1729 行)+QueryEngine.ts(1295 行) 双层结构；tools/ 44 个工具目录与官方工具五分类吻合；权限在 hooks/toolPermission/；compact 在 services/compact/；产品级 Hooks 引擎位置**待验证**（src/hooks/ 多为 React hooks）
- 待办沉淀：Step 12 时先定位 Hooks 引擎真身

## 思考题存档（下次开始先对答案）

- S0-1：cc-haha 为什么把 loop 拆成 query.ts + QueryEngine.ts 两层？你会怎么拆？
- S0-2：为什么 BashTool 与 PowerShellTool 分立成两个工具，而不是一个带 shell 参数的工具？
- S1-1：Message.Content 现在是 string，Step 4 回填 tool_result 时必须变成块数组——为什么 API 允许 user 消息的 content 既可以是字符串也可以是块数组？
- S1-2：把 max_tokens 改成 64 再跑一次，观测行的 stop_reason 会变成什么？为什么？
