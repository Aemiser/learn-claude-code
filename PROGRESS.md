# 进度（每步必更）

## 当前状态

- 阶段：Phase 2 已开始
- 已完成：Phase 0 校准、Phase 1 计划、**Step 0（cc-haha 结构考古）**
- 下一步：Step 1 hello-loop（最小 Go 问答，裸 HTTP 调 Magpie /v1/messages）
- 阻塞项：无

## 决策日志

| 日期 | 决策 |
|---|---|
| 2026-11-19 | Phase 0 证伪「CC 开源」；三条腿模型确立 |
| 2026-11-19 | 引入 reference/cc-haha（B 线主对照），三条红线：只读/不复制/标签纪律 |
| 2026-11-19 | 技术栈 Go；系统 1.22 不动，项目内用 1.25 工具链（GOTOOLCHAIN=auto + goproxy.cn） |
| 2026-11-19 | 模型端点=本地 Magpie 127.0.0.1:3425 /v1/messages（Anthropic 兼容，免鉴权）；默认 auto/glm-5.3 |

## Step 记录

### Step 0 ✅（2026-11-19）
- 产出：notes/cc-haha-map.md（L0-L8 锚点映射+证据）、PLAN.md、ARCHITECTURE.md、PROGRESS.md、.gitignore；git init（本步首次提交）
- 关键发现：cc-haha 的 query.ts(1729 行)+QueryEngine.ts(1295 行) 双层结构；tools/ 44 个工具目录与官方工具五分类吻合；权限在 hooks/toolPermission/；compact 在 services/compact/；产品级 Hooks 引擎位置**待验证**（src/hooks/ 多为 React hooks）
- 待办沉淀：Step 12 时先定位 Hooks 引擎真身

## 思考题存档（下次开始先对答案）

- S0-1：cc-haha 为什么把 loop 拆成 query.ts + QueryEngine.ts 两层？你会怎么拆？
- S0-2：为什么 BashTool 与 PowerShellTool 分立成两个工具，而不是一个带 shell 参数的工具？
