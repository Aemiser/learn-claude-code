# 进度（每步必更）

## 当前状态

- 阶段：Phase 2 进行中
- 已完成：Phase 0 校准、Phase 1 计划、Step 0–5、**Step 6（Edit/Glob/Grep，里程碑 M2 + 复盘#2）**
- 下一步：Step 7 Bash 工具 + 并发工具调用（L4.3 + L3.5）——agent 获得执行命令的能力
- 阻塞项：无

## 决策日志

| 日期 | 决策 |
|---|---|
| 2026-11-19 | Phase 0 证伪「CC 开源」；三条腿模型确立 |
| 2026-11-19 | 引入 reference/cc-haha（B 线主对照），三条红线：只读/不复制/标签纪律 |
| 2026-11-19 | 技术栈 Go；系统 1.22 不动，项目内用 1.25 工具链（GOTOOLCHAIN=auto + goproxy.cn） |
| 2026-11-19 | 模型端点=本地 Magpie 127.0.0.1:3425 /v1/messages（Anthropic 兼容，免鉴权）；默认 auto/glm-5.3 |
| 2026-11-19 | 工作约定：代码在 go/ 下；每步先写 notes/steps/step-XX.md 前置导读；每步一个 commit |
| 2026-11-19 | 新规则 R16：建立 METHODOLOGY.md，持续收集学习规则与想法，迭代成可复用方法论/skill；每步第8段检查更新 |
| 2026-11-19 | 课程调序：S2↔S3（学习者想先看 SSE 流式），总量与依赖不变 |

## Step 记录

### Step 6 ✅（2026-11-20）—— 里程碑 M2
- 产出：go/tools.go 增 Edit/Glob/Grep（loop/model 依旧零改动）、notes/steps/step-06.md + edit-flow 双主题 SVG（首次 workflow 模式，2 轮布局修复）
- 验证矩阵：M2 剧本 Write→Edit→Grep 四连发 ✅；Edit 唯一匹配+行号片段 ✅；**多匹配拒绝实测触发**（文案与 cc-haha FileEditTool.ts:336 逐字一致）✅；read-first 对 Edit 生效 ✅；模型自避歧义（带上下文分两次编辑）✅；Glob 双星 ✅
- 意外发现：模型在多匹配测试中两次自发规避歧义、第三次被强制裸 old_string 时 thinking 里**预判会失败仍执行**——约束已内化进模型决策
- 未测：0 处匹配分支（与多匹配同构，代码审查覆盖）；Glob 排序细节
- 测试方法论教训：验证 agent 行为分支时，指令要够「死板」，否则模型会自发绕开待测路径

## 复盘点 #2（Step 6 后，R13）
- 🟡 已实装：L0.2、L2.1–2.4、L3.1–3.4、L4 全段（4.1/4.2a/4.2b/4.4）、L6.1（内存）
- ⬜ 空壳：L1 全部、L3.5 并发、**L5 全部（当前最大空壳：agent 能改任何文件却无人把关）**、L6.2/6.3、L7、L8
- M2 达成。路线：S7（Bash+并发）→ S8（L5 权限）正好补最大空壳
- METHODOLOGY v0.5（见 C 区）

### Step 5 ✅（2026-11-20）
- 产出：go/tools.go 重写（Read/Write 入表，loop/model 零改动）、notes/steps/step-05.md（首张 R18 archify SVG：notes/assets/tool-registry-{light,dark}.svg）
- 验证：vet 过；三测全过——读 PLAN.md 出行号化结果+准确总结（in=2507 成本可见）；Write 新文件落盘；read-first 拦截后模型自纠正（先 Read 再 Write 成功）
- B 线证据入册：工具名 PascalCase、2000 行上限、cat -n 格式、read-first 防护均出自 cc-haha FileRead/WriteTool/prompt.ts【社区证据】
- 遗留：路径越界无防护（刻意推迟 S8/L5）；FILE_UNCHANGED_STUB 未做；Edit 未做（S6）

### Step 4 ✅（2026-11-19）—— 全课转折点
- 产出：go/tools.go（L4.1 Tool 接口+注册表+get_time）、go/loop.go（L3.1–3.4 loop）、go/model.go 重写（tool_use 流式解析+Message 二态+ContentBlock）、go/main.go（调 runLoop+回滚锚点）、notes/steps/step-04.md
- 验证：go vet 通过；「查纽约时间」→ 工具调用+结果+第二轮思考+最终回答（EST 正确），轮数=2，history=4
- 关键发现（证据先行）：tool_use 流式参数经 input_json_delta 的 partial_json 片段拼接，非一次性 JSON；cc-haha 教训「stop_reason 不可靠」→ 自记 toolUseSeen 作判定信号
- 教学简化：thinking 不入历史（真实 CC 在 trajectory 内保留）；流结束后才执行工具（真实为 streamingToolExecutor）
- 遗留：L3.5 并发未做（S7）；未知工具名回 is_error 已处理但未实测（S4-2 思考题）

### Step 3 ✅（2026-11-19）
- 产出：go/model.go（L2 独立：SSE 解析+重试+StreamHooks 契约）、go/main.go 重写（REPL+history 回滚）、notes/steps/step-03.md
- 验证：go vet 通过；两连问记忆测试通过（答出「小明」，history 2→4，in 37→91 随历史增长）
- 里程碑 **M1** 达成：流式多轮 CLI 聊天机
- 设计决定：中断/失败回滚整个轮次（保 user/assistant 交替）；只在流未开始时重试；StreamHooks 隔离 L2↔L0
- 遗留：history 无修剪（S9 compact）、无持久化（S9）、tool_use 流式形态未遇（S4）、重试路径未遇真实故障（自测法：停 Magpie 观察 4 次 retry 日志）

## 复盘点 #1（Step 3 后，R13）
- 🟡 已实装：L0.2、L2.1/2.2/2.3/2.4（观测级）、L6.1（内存）、L3.4（中断种子）
- ⬜ 空壳：L1 全部、**L3 主体（loop 未转——当前最大空壳）**、L4/L5/L7/L8 主体
- 结论：S4 起进入 agent 本体；METHODOLOGY v0.2（R17 升格 + 「证据先行」入观察）

### Step 2 ✅（2026-11-19）
- 产出：go/main.go 重写为 stream-loop（~175 行，v1 单轮版在 git 历史）；notes/steps/step-02.md
- 验证：管道实测「用一句话说明什么是 SSE」→ thinking/text 双段流式，stop_reason=end_turn，in=17 out=436
- 关键实测发现：message_start.usage.input_tokens 是占位 0，真值在 message_delta——账目解析以后者为准
- 遗留：无 history/无重试（S3）；tool_use 的流式形态 input_json_delta 尚未遇到（S4）；Ctrl+C 在提示符空闲时仍默认杀进程（可接受，S9 会话持久化前无状态可丢）

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
- S2-1：data 行的 JSON 里已经带了 type 字段，SSE 协议为什么还要有 "event:" 行？（提示：想想通用 SSE 客户端和带外路由）
- S2-2：Ctrl+C 中断后我们直接丢弃了半截回复。真实 CC 的 "Interrupt and steer" 允许打断后补一句话继续——猜猜它把半截回复怎么处理了？（S3 有 history 后我们同样要面对：半截 assistant 消息进不进 history）
- S3-1：为什么只在「流未开始」时重试？流中途失败后重试，用户会看到什么 bug？
- S3-2：history 无限增长有哪两个代价？修剪应发生在哪一层、依据什么信息决策？（提示：观测行 in=37→91）
- S4-1：为什么 tool_result 必须放在 role:"user" 的消息里，而不能放 assistant？（提示：API 交替约束 + 语义：谁在"提供信息"）
- S4-2：模型若调用一个不存在的工具名（如编造 read_file），loop 会怎样？为什么回 is_error 比直接崩溃更好？
- S5-1：Read 结果为什么要带 cat -n 行号？给谁看的？（提示：S6 的 Edit 靠什么锚定改动位置）
- S5-2：read-first 防护防的是什么事故？如果没有它，一次「凭想象覆盖」会毁掉什么？
- S6-1：Edit 在 >1 匹配时拒绝执行（要求补上下文），而不是像 sed 那样默认替换第一处——这个「不方便」为什么是安全关键？
- S6-2：真实 Grep 用 ripgrep（默认尊重 .gitignore），我们用 Go regexp 全量遍历——在 reference/cc-haha 这种带 .gitignore 的大仓库里搜，两者结果会差在哪？这个差异何时会误导模型？
