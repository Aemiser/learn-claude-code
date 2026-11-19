# Go 语言底层函数篇（问答沉淀 · 索引）

> **本目录定位**：学习 `go/model.go` 过程中提出的 Go 语言/标准库问题，一问一答整理成知识点文件。
> 与 [messages-api-fields.md](../messages-api-fields.md)（API 字段契约）互补：那边是「协议」，这边是「语言」。
>
> **整理纪律**：每篇 = 原问题 + 当时选中的代码 + 完整解答 + 一句话总结。解答基于 Go 标准库通用事实。

## 分类索引

| 分类 | 文件 | 原问题 | 关联代码 |
|---|---|---|---|
| 01-类型系统 | [函数类型与回调.md](01-类型系统/函数类型与回调.md) | `StreamHooks` 里的 `func(...)` 字段是什么 | model.go:54-59 StreamHooks |
| 02-标准库io | [io与bufio的区别.md](02-标准库io/io与bufio的区别.md) | io 和 bufio 都在哪层缓存？TCP 缓存是谁在读？ | model.go:110 vs 116 |
| 03-错误处理 | [apiError错误分支与errors.As.md](03-错误处理/apiError错误分支与errors.As.md) | 非 200 分支在干嘛；`errors.As(err, &ae)` 是什么 | model.go:109-112 / 168-174 |
| 04-context与取消 | [context与signal.NotifyContext.md](04-context与取消/context与signal.NotifyContext.md) | `signal.NotifyContext(context.Background(), os.Interrupt)` 是什么意思 | main.go:63 |

## 待补充（后续提问继续入册）

- goroutine / channel（S4 agent loop 会用到）
- `select` 语句细节（chatWithRetry 里已有：`select { case <-ctx.Done(): ... }`，04 篇已初步涉及）
- 接口与 duck typing（`error` 接口已初步接触，见 03 篇）
