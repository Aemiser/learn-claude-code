# Step 3 前置导读 · 模块化 + 多轮记忆 + 重试（写于代码之前）

## 现在在做什么
1. **拆文件**：L2 模型调用层独立为 go/model.go，main.go 只剩 REPL（L0）与 history（L6.1 雏形）；
2. **长记忆**：主循环维护 `history []Message`，每轮把 user 与 assistant 消息成对追加；
3. **抗造**：chatWithRetry——网络错误与 HTTP 429/5xx 指数退避重试（最多 4 次，500ms×2ⁿ）。

## 有什么用（三个故障模式倒逼）
- 无记忆 = 金鱼：跨轮任务全忘；
- 无重试 = 脆皮：一次限流/抖动就崩，真实 CC 必须扛住（cc-haha 专门有 services/api/withRetry.ts
  【社区证据（cc-haha）】）；
- 不分层 = S4 的 agent loop 没地方放：L3 必须站在稳定的 L2 接口之上。

## 架构位置
- L2.1 请求构造**补完**：请求体现 `{system, messages, max_tokens, stream}` 全字段
- L2.3 错误重试（新增）
- L6.1 history 管理（雏形：仅内存、无修剪——修剪是 S9 compact 的课题）
- 层间契约新面孔：**StreamHooks**（OnBlockStart/OnDelta/OnBlockStop）——L2 只报告事件，
  渲染归调用方。上一步「model 层直接 fmt.Print」的分层臭味就此清除。

## 关键设计决定
1. **中断/失败回滚整个轮次**（S2-2 我们的答案）：悬空的 user 消息会破坏 user/assistant
   交替约束【合理推断；验证法：连发两条 user 看是否 400】；
2. **只在「流未开始」时重试**：已打印半截回复后重试会导致重复输出（真实 SDK 同样头疼），
   教学简化：中途断流直接报错回滚；
3. **system prompt 落位**：一条终端助手人设，L1.3（CLAUDE.md 注入）未来会替换这里。

## 刻意不做（后续哪步做）
tools 字段与 tool_use 块（S4）｜history 修剪/compact（S9）｜baseURL/model 配置化（S10）
｜Go package 拆分（S5 工具层出现时再议，现在单 package 多文件够用）

## 运行与验收
cd go; go run .
连问两轮：「我叫小明，请记住」→「我叫什么名字？」第二论应答出小明；
观测行出现 `history=N 条` 且每轮 +2；把 Magpie 停掉再问 → 观察 4 次重试日志后报错、
history 不增长，重启 Magpie 后无需重启程序即可继续。

## 与真实 CC 对照
- withRetry 同职责【社区证据（cc-haha）：services/api/withRetry.ts】
- 官方 Go SDK (anthropic-sdk-go) 亦内置重试【官方事实：仓库存在，Apache-2.0】——
  我们的 30 行手写版就是它的教学镜像
