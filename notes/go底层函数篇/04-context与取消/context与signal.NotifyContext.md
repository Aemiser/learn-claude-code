# context 与 signal.NotifyContext（Ctrl+C 刹车链路）

> 来源：2026-11-19 学习 Step 3 时提问。当时选中 main.go:63。
> 分类：04-context与取消 ｜ 关联：model.go 的 `NewRequestWithContext`、`chatWithRetry` 的 select

## 当时选中的代码（main.go:63-65）

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
res, err := chatWithRetry(ctx, history, consoleHooks())  // 把刹车交给下层
stop()                                                    // 本轮结束，拆掉刹车
```

## 问题

这是什么意思？

## 解答

一句话：**给「本轮请求」装一个 Ctrl+C 刹车**。

### 一、先认识 `context`：一张「取消令牌」

`context.Context` 不是业务数据，它只做一件事：**在多个函数/协程之间广播「该停了」**。

- `context.Background()` 是一个空的、永不取消的根 context——所有 context 的起点。
- 核心是一个 `Done()` channel：**一旦被取消，这个 channel 就关闭**。任何拿到 ctx 的下层代码，只要监听 `<-ctx.Done()`，就能立刻知道「上面不要结果了，赶紧收手」。

### 二、`signal.NotifyContext`：把「操作系统信号」翻译成「context 取消」

- `os.Interrupt` 就是**终端里按 Ctrl+C** 发出的中断信号（Unix 上叫 SIGINT）。
- `signal.NotifyContext(parent, sig)` 的语义：**「派生一个新 context；一旦进程收到 `sig` 这个信号，就自动把这个新 context 取消掉」**。
- 返回两个值：
  - `ctx`：那个会被信号取消的 context；
  - `stop`：一个函数，调用它会**撤销这个订阅**（不再因信号取消该 ctx），并恢复信号默认行为（Ctrl+C 重新变成「杀掉进程」）。

**类比**：把「Ctrl+C 这个物理按键」接到了「ctx 这个电路开关」上。

### 三、刹车怎么传到最底层（整条链路）

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="../assets/cancel-chain-dark.svg">
  <img alt="Ctrl+C 取消链路：终端触发 → ctx 被取消（Done channel 关闭）→ HTTP 请求被取消 → Scanner 中断 → 返回 context.Canceled → main 回滚本轮消息" src="../assets/cancel-chain-light.svg" width="891">
</picture>

代码对应：

| 位置 | 作用 |
|---|---|
| model.go:99 `http.NewRequestWithContext(ctx, ...)` | 把 ctx 绑到请求上，取消时连接被掐断 |
| model.go:156-161 读流出错时 `ctx.Err() == context.Canceled` | 区分「用户取消」与普通断流 |
| model.go:191-193 重试等待的 `select` 监听 `ctx.Done()` | 退避睡到一半按 Ctrl+C 也能立刻醒 |
| main.go:69-70 识别取消 | 回滚本轮消息 |

### 四、为什么每轮都新建，而不是全局建一个？

设计目标是 **「Ctrl+C 只中断这一轮，进程不死」**（Step 2 的核心体验）。全局建一个的话，第一次 Ctrl+C 就会永久取消后续所有请求。
每轮新建、用完 `stop()`，语义就是「**只给这一轮上一个临时刹车**」；`stop()` 之后 Ctrl+C 恢复默认行为——提示符空闲时按 Ctrl+C 照样能退出程序。

### 五、`stop()` 为什么必须调用？

1. **释放订阅**：不调用，内部注册会一直挂着；REPL 每轮注册一次会不断累积。
2. **恢复默认信号行为**：下一轮空闲时按 Ctrl+C 不会被过期 ctx 吃掉。

> 更常见写法是 `defer stop()`（函数返回时执行）；这里在 `chatWithRetry` 返回后立刻调用，效果等价且更明确「本轮已结束」。

## 一句话总结

> `signal.NotifyContext(context.Background(), os.Interrupt)` = **派生一个「按 Ctrl+C 就取消」的 context**；它通过 HTTP 请求、重试等待、流式读取一路传递，实现「**只中断本轮、不杀进程**」；`stop()` 用完拆除，恢复 Ctrl+C 默认行为。
>
> 顺带：这种「ctx 一路往下传」是 Go 标准做法——Step 8 权限确认、Step 13 Subagent 会复用同一套取消机制。
