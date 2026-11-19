# Step 4 前置导读 · 第一次 tool loop（写于代码之前）

> 全课转折点：从「chatbot」到「agent」。模型第一次不是"说完"，而是"要求做事、拿到结果、再说"。

## 现在在做什么
1. 定义 L4.1 的 `Tool` 统一抽象 + 注册表 + 一个 `get_time` 玩具工具；
2. 实现 L3 的 agent loop：`stop_reason=tool_use` 时 → 解析 tool_use 块 → 执行 → 以
   `tool_result`（role:user）回填 → 再请求，直到模型不再请求工具；
3. 契约升级：Message.Content 从 string 变二态（string 或 块数组）；模型层学会解析
   tool_use 块与 input_json_delta 流。

## 有什么用
没有这层，模型只会"说"，不会"做"——它无法查时间、读文件、跑命令。loop 是 harness 与
模型的握手协议：模型输出**意图**（tool_use），harness 执行并回报**事实**（tool_result）。

## 架构位置
- L3.1 循环骨架（新增 loop.go）
- L3.2 解析 tool_use（content_block_start + input_json_delta 拼 JSON）
- L3.3 回填 tool_result（tool_use_id 配对；role 必须是 user；is_error 标记失败）
- L3.4 停止条件（maxRounds=10；另有 ctx 取消）
- L4.1 Tool 接口 + 注册表（新增 tools.go）

## 契约（一手实测，2026-11-19 curl 抓包）
```
content_block_start  → {content_block:{id:"call_…", name:"get_time", input:{}, type:"tool_use"}}
content_block_delta  → {delta:{partial_json:"{\"timezone\": ", type:"input_json_delta"}} × N
content_block_stop   → {index:N}
message_delta        → {delta:{stop_reason:"tool_use"}, usage:{…}}
```
非流式等价物：`content:[{id,name,input,type:"tool_use"}]`, `stop_reason:"tool_use"`。
**关键设计决定（cc-haha 教训）**：不信任 stop_reason（它"not always set correctly"
【社区证据（cc-haha）：query.ts:554】），改用**本轮是否出现 tool_use 块**作为判定信号。

## 回填契约（官方）
tool_result 必须：`type:"tool_result"` + `tool_use_id` 与对应 tool_use.id 严格配对；
放在 **role:"user"** 的消息里；失败时 `is_error:true`（让模型知道"没做成"，它会改口或
换策略）【官方事实：platform.claude.com how-tool-use-works】。

## 教学简化边界（诚实）
- **thinking 不入历史**：我们把模型两轮之间的推理块丢弃，模型下一轮重新推理。
  真实 CC 在 assistant trajectory 内保留 thinking【社区证据（cc-haha）：query.ts:158】。
  Magpie/GLM 上丢弃可正常work，故简化；以后可补。
- **流结束后才执行工具**：真实系统边流式边执行（cc-haha 的 streamingToolExecutor，
  query.ts:851）；我们等整轮流结束后统一执行——够教学用，S7 并发时再谈。

## 运行与验收
cd go; go run .
问「现在北京时间几点？用 get_time 查一下」→ 应看到：
思考 →（可能一段文字）→ 🔧 调用工具 get_time(timezone=Asia/Shanghai) → ✓ 工具返回 → 再思考 → 最终回答带时间
观测行 `轮数=2`（模型被调了两次）。
再问「那纽约呢？」→ 验证多轮里 history 已含 tool_result，模型能基于上一轮结果继续。

## 与真实 CC 对照
- loop 定义【官方事实：how-tool-use-works】
- 工具抽象 cc-haha `Tool.ts`（792 行，zod v4 + ToolUseBlockParam/ToolResultBlockParam）
  【社区证据（cc-haha）】——我们是它 ~30 行的 Go 镜像
- tool_result 构造 cc-haha `query.ts:127-141`【社区证据（cc-haha）】
