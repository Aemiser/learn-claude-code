# io 与 bufio 的区别（缓存在哪一层）

> 来源：2026-11-19 学习 Step 3 时提问。当时对照 model.go:110（io.ReadAll）与 model.go:116（bufio.Scanner）。
> 分类：02-标准库io

## 问题

io 和 bufio 的区别？是不是 io 从 TCP 缓存里读，bufio 是系统层还是读进进程里的缓存？

## 解答

先纠正关键误解——**`io` 和 `bufio` 都不在「TCP 缓存」里，它们俩全是跑在你进程里的 Go 代码**。真正碰内核 TCP 缓冲的，是更底下的 `net` 包。

### 一、缓存出现在三层，别混了

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="../assets/io-bufio-read-path-dark.svg">
  <img alt="io 与 bufio 读取分层图：你的代码 → bufio → io → resp.Body（net/http）→ net 包 → 内核 TCP 接收缓冲区" src="../assets/io-bufio-read-path-light.svg" width="690">
</picture>

*（图中实线灰箭头那条是响应路径；左侧 `io.ReadAll` 分支只用于收集错误体，≤512 字节。）*

所以：**bufio 是进程里的缓存（用户态）**。`io` 也不是「从 TCP 缓存读」，它只是个接口和工具箱。

### 二、`io` 包：接口 + 通用工具，不是数据源

`io` 包的核心是一个**接口**：

```go
type Reader interface {
    Read(p []byte) (n int, err error)  // 读一些字节填进 p，返回读了多少
}
```

只要某个东西实现了 `Read` 方法，它就是 `io.Reader`。**文件、网络连接、内存里的字符串、压缩流……全都是 `io.Reader`**。所以 `io` 根本不知道也不关心你后面是 TCP 还是硬盘。

- `io.ReadAll(r)`：循环调 `r.Read()`，把读到的字节不断追加进一个会动态变大的切片，直到读到 EOF。
- `io.LimitReader(r, 512)`：包一层，让读满 512 字节后假装 EOF。

它们**自己不加预读缓冲**，老老实实一次一次调 `Read`。

### 三、`bufio` 包：在 `io.Reader` 之上加一层进程内预读缓冲

`bufio.NewScanner(resp.Body)` 干的事：**把 `resp.Body` 包起来，内部维护一个 `[]byte` 缓冲**。价值两点：

1. **减少 syscall**。没缓冲时读一个字节就调一次 `Read`（=一次系统调用），很慢。`bufio` 一次性从底层 `Read` 一大块（比如 64KB）进自己的缓冲，后续小读都从进程内缓冲拿，几乎零开销。
2. **提供按行/按分隔符读**。`Scanner.Scan()` 自动按 `\n` 切行；裸的 `io.Reader` 只懂「给我一堆字节」。

### 四、model.go 两处用法的对比

```go
// ① 出错分支: 用 io 包
b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))

// ② 正常分支: 用 bufio 包
sc := bufio.NewScanner(resp.Body)
sc.Buffer(make([]byte, 64*1024), 1024*1024)
for sc.Scan() { ... }
```

**两边读的是同一个 `resp.Body`**，差别只在「怎么组织读」：

| | ① `io` | ② `bufio` |
|---|---|---|
| 读法 | 一次性读到底（上限 512B） | 一行一行地读 |
| 适合 | 「把结果整个拿回来再说」 | 「边来边处理」的流式场景 |

SSE 是「服务器一条一条推事件」，必须**边读边处理**、按行切，所以用 `bufio.Scanner`；错误体只有一小段说明文字，直接 `io.ReadAll` 一把抓回来省事。

### 五、一个细节

`resp.Body` 内部其实已经自己包了一层 `bufio.Reader`（HTTP 要先读响应头，需要预读缓冲）。即：**TCP 缓冲 → net 包 → net/http 已加一层 bufio → 你又用 bufio.Scanner 包一层 → 你的代码**——缓冲可能叠好几层。

## 一句话总结

> **`io` = 接口和通用读函数（"读谁"由传进来的 Reader 决定）；`bufio` = 给 Reader 套一层进程内缓冲（"怎么读得高效/方便"）。TCP 缓存在内核，由 `net` 包经 syscall 读进来，和这俩都不在同一层。**
