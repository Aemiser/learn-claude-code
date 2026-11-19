# Step 1 前置导读 · hello-loop（写于代码之前）

## 现在在做什么
第一次让「我们的程序」和「模型」说上话。一个单文件 Go REPL：读入消息 → 裸 HTTP 调
Magpie 的 `/v1/messages`（Anthropic 兼容）→ 打印回复和一行观测（stop_reason / token 数）。

## 有什么用
1. 验证端点契约真的能在 Go 里跑通（Phase 0 只用 curl 验过）；
2. 建立 Go 工程骨架（go/ 目录 + go.mod + 工具链隔离）；
3. 打通 L2.1 最小切片——之后 13 步全部长在这次通话之上。

## 架构位置
- L0.2 REPL 与渲染（最小版：bufio.Scanner 读入 + fmt 打印，无 TUI）
- L2.1 请求构造（单轮、非流式；system/tools 字段本步不出现）
- 其余层全部不存在——**这是刻意的**，防止跨层堆料。

## 数据流
你键入 → for 循环(L0.2) → ask() 组 JSON POST(L2.1) → Magpie →
Response{content[], stop_reason, usage} → 挑出 type=="text" 的块 → 打印 + 观测行

## 契约速览（Phase 0 实测响应，精简）
```json
{
  "content": [
    {"type": "thinking", "thinking": "…推理文字…", "signature": ""},
    {"type": "text", "text": "ok"}
  ],
  "stop_reason": "end_turn",
  "usage": {"input_tokens": 17, "output_tokens": 41}
}
```
注意三点：content 是**块数组**（不是纯字符串）；stop_reason 是 loop 的方向盘
（end_turn=正常说完 / max_tokens=被截断）；usage 是 L2.4 成本核算的原料。

## 刻意不做（后续哪步做）
多轮 history(S2)｜system prompt(S2)｜重试(S2)｜streaming、thinking 展示(S3)｜工具(S4)

## 与真实 CC 对照
- 官方：CC 是 agentic harness——「models that reason and tools that act」的壳
  【官方事实：code.claude.com/docs/en/how-claude-code-works】。本步 = 壳的嘴和耳朵。
- cc-haha：同职责在 src/services/api/claude.ts【社区证据（cc-haha）】。

## 运行
cd go; go run .（首次会自动下载 go1.25 工具链；中文乱码先执行 chcp 65001）

## 验收标准
中文问答正常返回，且观测行显示 stop_reason=end_turn。
