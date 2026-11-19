# Messages API 请求/响应字段参考（L2 契约 · 知识点文档）

> **本文定位**：把 `go/model.go` 里那些 `json:"xxx"` 标签背后的字段含义讲清楚——一张「字段→含义→证据」对照表。
> 覆盖三步用到的东西：请求体（L2.1）、非流式响应（S1）、流式 SSE 事件（S2/S3）。
> 你敲代码时遇到看不懂的字段，先来查这张表。
>
> **证据纪律**：本端点不是官方 Anthropic 服务器，而是本地 Magpie 的「Anthropic 兼容」实现。
> 因此字段**名字与通用语义**以官方 Messages API 为准【官方事实】；**本端点实际吐出的序列/取值**以
> `notes/steps/step-02.md` 的 curl 抓包为准【实测抓包】。两者若不一致，以实测为准（差异处会点名）。

---

## 0. 一句话总览

一次对话请求，就是往 `/v1/messages` 发一个 JSON（`POST`），服务器回一个 JSON（非流式）
或一长串 `event:` + `data:` 行（流式 SSE）。两个方向共同的核心概念是 **content block（内容块）**：
回复不是一坨字符串，而是**一块一块**拼起来的数组，每块有自己的 `type`（`text` / `thinking` / `tool_use`…）。

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/request-response-dark.svg">
  <img alt="请求与响应形态：POST /v1/messages → stream 为真走 SSE 事件串，缺省走全量 JSON，最后由客户端解析" src="assets/request-response-light.svg" width="950">
</picture>

---

## 1. 请求字段（L2.1 · `chatStream` 组出来的 JSON）

| 字段 | 类型 | 必填 | 含义 | 本项目取值 | 证据 |
|---|---|---|---|---|---|
| `model` | string | ✅ | 用哪个模型。可以是单一模型 id，也可以是路由组（如 `auto/glm-5.3`） | `auto/glm-5.3` | 官方事实 |
| `max_tokens` | int | ✅ | 生成**最多**这么多 token 就停。是硬上限，超了会被截断（stop_reason=`max_tokens`） | `4096` | 官方事实 |
| `messages` | array | ✅ | 对话历史，按时间顺序；每条是 `{role, content}` | 见下表 | 官方事实 |
| `system` | string | 否 | 系统提示词（人设/规则），与 `messages` 分离 | 终端助手人设 | 官方事实 |
| `stream` | bool | 否 | `true` 则走 SSE 逐 token 推；缺省 `false` 则一次性回全量 JSON | `true` | 官方事实 |
| `temperature` | number | 否 | 采样温度 0~1，控制随机性 | 未用（缺省） | 官方事实 |
| `stop_sequences` | array | 否 | 自定义停止词，命中即停 | 未用 | 官方事实 |
| `tools` | array | 否 | 工具定义（S4 引入） | 未用（S4） | 官方事实 |

### `messages` 数组里的一条（`Message` 结构体）

| 字段 | 含义 | 备注 |
|---|---|---|
| `role` | `"user"` 或 `"assistant"` | 谁说的。必须**严格交替**（user→assistant→user…），否则报 400 |
| `content` | 字符串，或块数组 | 本项目现在用纯字符串；S4 回填 `tool_result` 时会演化成块数组（见 model.go 注释） |

---

## 2. 非流式响应字段（S1 的 `Response`）

> 这是 `stream=false` 时的返回形态（S1 用过；现在 S3 已切流式，但字段是理解块的入门）。

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

| 字段 | 含义 |
|---|---|
| `content` | **块数组**（不是纯字符串）。每个元素是一个 content block |
| `stop_reason` | 方向盘：这轮为什么停。见第 4 节枚举 |
| `usage` | 账目：`input_tokens` 输入、`output_tokens` 输出。L2.4 成本核算的原料 |
| `id` / `type` / `role` / `model` | 消息 id、类型（`"message"`）、角色（`"assistant"`）、用的模型 |

### content block 的 `type` 有哪些（重点记）

| type | 字段 | 含义 | 本项目何时遇到 |
|---|---|---|---|
| `text` | `text` | 正常回答文字 | 每轮都有 |
| `thinking` | `thinking` + `signature` | 推理模型的「思考过程」文字 | GLM-5.3 是 reasoning 模型，每轮都有 |
| `tool_use` | `id` `name` `input` | 一次工具调用 | **S4** 才会出现 |
| `redacted_thinking` | `data` | 被脱敏/隐藏的思考 | 本项目基本不遇到 |

> 关键认知：回复 = 若干个块按顺序排。典型是「一段 thinking + 一段 text」。
> 处理时必须遍历 `content` 数组按 `type` 挑，而不是直接拿整段字符串——S1 的 `挑出 type=="text"` 就是干这个。

---

## 3. 流式 SSE 事件字段（S2/S3 · `streamEvent` 结构体）⭐ 你选的这段

流式下，服务器不再一次回全量，而是按事件**推**过来。每个事件是两行：

```
event: <事件名>
data: <一段 JSON>
```

`model.go` 只解析 `data:` 行（因为 JSON 里自带的 `type` 字段已经能区分事件类型，`event:` 行是冗余的）。
`streamEvent` 这一个结构体被**所有事件复用**，靠 `Type` 字段做判别——这是「判别联合（discriminated union）」套路。

### 3.1 事件全序列（step-02 实测抓包）

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/sse-stream-sequence-dark.svg">
  <img alt="SSE 流式事件时序图：POST 请求后，message_start、thinking 块的 start/delta/stop、text 块的 start/delta/stop、message_delta、message_stop 依次推送" src="assets/sse-stream-sequence-light.svg" width="700">
</picture>

骨架就三件事：`message_start`/`message_stop` 包住整轮；每个块固定 `start → delta×N → stop`；
`stop_reason` 与 `usage` 真值只在 `message_delta`（`message_start` 里的 input_tokens 是占位 0）。

### 3.2 每个事件里 `streamEvent` 哪些字段有值

| 事件名（`Type`） | 有效字段 | 含义 |
|---|---|---|
| `message_start` | `Message.Usage.InputTokens` | 回合开始。⚠️ 这里的 input_tokens 是**占位 0**，真值在后面 |
| `content_block_start` | `Index` + `ContentBlock.Type` | 第 `Index` 个块开始了，它是什么类型（thinking/text/tool_use） |
| `content_block_delta` | `Index` + `Delta` | 第 `Index` 个块**追加了一小段增量**（逐 token） |
| `content_block_stop` | `Index` | 第 `Index` 个块结束了 |
| `message_delta` | `Delta.StopReason` + `Usage` | 方向盘 + 账目**真值**（这才是准的） |
| `message_stop` | （无） | 整个回合结束 |

### 3.3 `deltaInfo`（`Delta` 字段）——增量对象，也被复用

```go
type deltaInfo struct {
	Type       string `json:"type"` // 判别：这是哪种 delta
	Text       string `json:"text"`
	Thinking   string `json:"thinking"`
	StopReason string `json:"stop_reason"`
}
```

`Delta.Type` 的取值（重点记，S4 会多一个）：

| Delta.Type | 实际携带字段 | 含义 |
|---|---|---|
| `thinking_delta` | `Thinking` | 思考过程又吐了一个 token |
| `text_delta` | `Text` | 回答又吐了一个 token |
| `signature_delta` | （本项目忽略） | 思考块的签名增量（扩展思考用） |
| `input_json_delta` | （S4 才有，`partial_json`） | 工具调用的参数 JSON 逐片推（S4） |

> ⚠️ `Delta.StopReason` 只在 `message_delta` 事件里有值——它**不是** delta 类型的一种，
> 而是 `message_delta` 事件借用了同一个 `deltaInfo` 结构体来装 `stop_reason`。
> 这就是为什么注释写「仅 message_delta 携带」。

---

## 4. `stop_reason` 方向盘枚举（loop 的核心）

`stop_reason` 回答「模型这轮为什么停」，是 agent loop 决定「下一步干啥」的开关：

| 取值 | 含义 | 本项目何时遇到 |
|---|---|---|
| `end_turn` | 正常说完，自然收尾 | 普通问答 |
| `max_tokens` | 撞到 `max_tokens` 上限被截断 | 长回答被掐 |
| `tool_use` | 模型想调工具（停下来等你执行并回填 tool_result） | **S4** |
| `stop_sequence` | 命中自定义停止词 | 未用停止词，基本不遇 |
| `pause_turn` | 模型暂停等待更多输入（扩展思考/agent 场景） | 暂不遇 |
| `refusal` | 模型拒绝回答（安全护栏） | 触发安全策略时 |
| `interrupted` | 用户中途打断（Ctrl+C） | 实测会返回，见 step-02 验收 |

> 证据：`end_turn / max_tokens / tool_use / stop_sequence / pause_turn / refusal` 为官方枚举【官方事实】；
> `interrupted` 是本项目 Ctrl+C 实测返回值【实测抓包】，是否属于官方边界、还是 Claude Code/Magpie 客户端合成，待核实。

---

## 5. `usage` 账目字段

| 字段 | 含义 |
|---|---|
| `input_tokens` | 输入 token 数（含 system + 历史消息） |
| `output_tokens` | 输出 token 数（这轮生成的部分） |
| `cache_creation_input_tokens` / `cache_read_input_tokens` | 提示缓存的写入/命中 token（L2.4/S9 涉及，本项目暂不处理） |

> ⚠️ 流式里的坑（step-02 已标注）：`message_start` 里的 `usage.input_tokens` 是**占位 0**，
> 真值在 `message_delta` 的 `usage` 里。所以代码里 `ChatResult` 的 token 数是从 `message_delta` 拿的。

---

## 6. 字段 ↔ Go 结构体 映射速查（model.go）

| Go 字段 | `json` 标签 | 对应 JSON 键 | 出现在哪个事件 |
|---|---|---|---|
| `streamEvent.Type` | `type` | 事件名 | 所有事件 |
| `streamEvent.Index` | `index` | 块下标 | block 三兄弟 |
| `streamEvent.Delta` | `delta` | 增量对象 | content_block_delta / message_delta |
| `streamEvent.ContentBlock.Type` | `content_block` | 块类型 | content_block_start |
| `streamEvent.Message.Usage.InputTokens` | `message` | 回合起始 usage | message_start |
| `streamEvent.Usage` | `usage` | 账目真值 | message_delta |
| `deltaInfo.Type` | `type` | delta 类型 | 见 3.3 |
| `deltaInfo.Text` / `.Thinking` | `text` / `thinking` | 增量文字 | text_delta / thinking_delta |
| `deltaInfo.StopReason` | `stop_reason` | 方向盘 | message_delta |

> 为什么 Go 字段首字母大写、标签里是小写+下划线？——Go 的 `encoding/json` 默认要求字段导出（首字母大写），
> 但 JSON 约定是小写蛇形（`input_tokens`），所以用 `json:"input_tokens"` 显式指定映射。这是 Go 接 JSON 的标准写法。

---

## 7. 容易混淆的三个点（自查清单）

1. **块 vs 增量**：`content_block_start` 只告诉你「这是什么类型的块」，**文字在 `content_block_delta` 里逐片来**。
   不要指望 start 事件里就有正文。
2. **`streamEvent.Type`（事件名） vs `Delta.Type`（增量类型）**：两个都是 `type`，但层级不同——
   外层是事件（message_start/…），内层 `delta.type` 是增量种类（text_delta/thinking_delta）。
3. **占位 usage**：`message_start` 的 input_tokens 是 0，别信它；信 `message_delta` 的 `usage`。

---

## 8. 官方锚点（三腿对照的位置）

- **官方事实**：Anthropic Messages API 参考（`docs.anthropic.com` / `platform.claude.com` 的 Messages 文档）。
  本项目端点名为 `/v1/messages`，即这套 API 的兼容实现。
- **实测抓包**：`notes/steps/step-02.md` 记录了 2026-11-19 curl 抓到的完整 SSE 序列（权威、优先信它）。
- **社区证据（cc-haha）**：cc-haha 消费同一套 SSE 的位置在 `src/services/api/claude.ts`（另见 `cli/transports/ccrClient.ts`）。
