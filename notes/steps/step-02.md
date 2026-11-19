# Step 2 前置导读 · stream-loop（写于代码之前）

> 调序说明：你说想先看 SSE 流式——原 Step 2（模块化+多轮+重试）与 Step 3 对调，总量不变。

## 现在在做什么
把 Step 1 的「等全量再打印」改成 SSE 流式：thinking（暗色）与回答（正常色）逐 token 蹦出；
回答期间 Ctrl+C 只中断本轮、进程不死（L3.4 中断机制的第一颗种子）。

## 有什么用
1. 体验：terminal agent 的生命感来自逐字输出；
2. 机制：长任务里用户必须实时看到模型行为才能纠偏——真实 CC 的
   "Interrupt and steer"【官方事实：how-claude-code-works】就建立在流式之上；
3. 技术底座：SSE 是「服务器单向推、客户端按行读」的最小形态，后面 MCP 的 HTTP 传输
   同族（本课不展开）。

## 架构位置
L2.2 streaming（最小实现）。L0.2 沿用；仍无 history/重试/模块化（全部留给 S3）。

## 契约：Magpie 实测 SSE 事件序列（2026-11-19 curl 抓包）
```
HTTP 200, content-type: text/event-stream
event: message_start        → usage.input_tokens=0（占位！真值在最后）
event: content_block_start  → {index:0, content_block:{type:"thinking"}}
event: content_block_delta  → {delta:{type:"thinking_delta", thinking:"…"}} × N
event: content_block_stop   → {index:0}
event: content_block_start  → {index:1, content_block:{type:"text"}}
event: content_block_delta  → {delta:{type:"text_delta", text:"…"}} × N
event: content_block_stop   → {index:1}
event: message_delta        → {delta:{stop_reason}, usage:{in/out 真值}}
event: message_stop
```
解析策略：只认 `data:` 行（type 字段自描述，`event:` 行冗余可跳过）；
块类型由 content_block_start 决定、增量由 content_block_delta 携带、账目以 message_delta 为准。

## 刻意不做（后续哪步做）
多轮 history、错误重试、拆 model.go（S3）｜tool_use 块的流式形态（S4 会遇到：
它走 content_block_start + input_json_delta，本步类型枚举里留了注释）

## 与真实 CC 对照
cc-haha 消费 SSE 的位置：src/services/api/claude.ts（另见 compact.ts、cli/transports/ccrClient.ts）
【社区证据（cc-haha）】。官方 Messages API 流式为标准能力（本步以实测抓包为准）。

## 运行
cd go; go run .   输入问题回车，看流式；回答中 Ctrl+C 中断本轮；exit 退出。
（暗色 ANSI 需要 Windows Terminal；乱码先 chcp 65001）

## 验收
逐字输出可见；thinking 与回答两段分明；中断后回到提示符可继续问；
观测行 stop_reason=end_turn / interrupted。
