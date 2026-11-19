# apiError 错误分支与 errors.As（自定义错误 + 错误链）

> 来源：2026-11-19 学习 Step 3 时两次提问（① model.go:109-112 非 200 分支；② model.go:170 `errors.As(err, &ae)`）。
> 分类：03-错误处理 ｜ 关联：messages-api-fields.md 的 429/5xx 重试语义

## 问题一：非 200 分支在干嘛（model.go:109-112）

```go
if resp.StatusCode != http.StatusOK {
    b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
    return ChatResult{}, &apiError{Status: resp.StatusCode, Body: string(b)}
}
```

**意思：请求返回了非 200 状态码时，把错误信息抓出来并包装成错误返回。**

### ① 判断成功与否

`http.StatusOK` 是 Go 内置常量，值就是 `200`。非 200 就进错误处理：400（参数错）、429（限流）、500/502/503（服务器问题）等。

### ② 读取响应体里服务器给的错误信息

从里往外读：

| 部分 | 作用 |
|---|---|
| `resp.Body` | 响应体。出错时服务器在里面放说明，如 `{"error":"model not found"}` |
| `io.LimitReader(resp.Body, 512)` | 包一层「**最多只读 512 字节**」的限制 |
| `io.ReadAll(...)` | 把（受限后的）内容全部读出来，得到 `[]byte` |
| `b, _ :=` | `io.ReadAll` 返回两个值；第二个是 error，用 `_` **丢弃**（Go 不允许声明不用的变量） |

**为什么要 LimitReader 限 512 字节？** 出错时响应体可能很大甚至异常。只需要错误原因那一点点信息，读到 512 字节就停，防止内存被塞爆。

### ③ 返回空结果 + 带状态码的错误

```go
return ChatResult{}, &apiError{Status: resp.StatusCode, Body: string(b)}
```

- `ChatResult{}`：空结构体（没成功，没有回复文本和 token 数）
- `&apiError{...}`：`apiError` 是自定义错误类型（model.go:38-43），实现了 `Error() string` 方法，所以**满足 `error` 接口**，可直接当 error 返回

**为什么记录 Status 和 Body？** 上层要靠 Status 判断能不能重试——`retryable` 的逻辑：4xx 是参数错重试无意义；429/5xx 是服务器临时抽风值得重试。

## 问题二：`errors.As(err, &ae)` 是什么（model.go:170）

```go
func retryable(err error) bool {
    var ae *apiError          // 先准备一个空指针，等着被填
    if errors.As(err, &ae) {  // 问：err 这条错误链里，有没有 *apiError 类型？
        return ae.Status == 429 || ae.Status >= 500
    }
    return true
}
```

### 它在解决什么问题

`chatStream` 出错时返回的是 `&apiError{...}`，但函数签名是 `error` 接口——到 `retryable` 手里只有一个 `error`，**不知道是不是 `apiError`**。而读 `Status` 字段需要拿回具体类型，所以要做**类型断言**。

### 逐参数解释

| 参数 | 含义 |
|---|---|
| `err` | 要检查的错误 |
| `&ae` | 目标类型的**指针的指针**（`ae` 是 `*apiError`，`&ae` 是 `**apiError`）。匹配成功时 `errors.As` 把真正的错误对象**回填进 `ae`** |
| 返回值 | `true` = 找到了，`ae` 已被填上；`false` = 链里没有，`ae` 保持 nil |

`ae` 必须先声明好再传地址——`errors.As` 是通过指针回填结果的。

### 为什么不用直接强转 `err.(*apiError)`？

在本项目里直接断言够用（错误都是刚造出来直接返回的）。但 `errors.As` 多一个关键能力：**沿错误链一路往下找**。

Go 的错误可以层层包裹：`fmt.Errorf("请求失败: %w", err)`——外层错误内部藏着原始错误，形成一条链（`%w` 就是「把底层错误包进来」）。直接断言只能看最外层；`errors.As` **解开每一层包装**逐个检查，链上任何一处是 `*apiError` 都能找到。

### 姊妹函数 `errors.Is`

`chatWithRetry` 里也在用（model.go:186）：

```go
if errors.Is(err, context.Canceled) || ...   // 判断错误链里有没有"某个特定值"
```

| 函数 | 找什么 | 类比 |
|---|---|---|
| `errors.Is` | 链里有没有**某个具体的错误值**（如 `context.Canceled`） | 问「你是不是这一张票？」 |
| `errors.As` | 链里有没有**某个类型的错误**，顺便把对象给你 | 问「你是不是这一类人？是的话证件给我看看」 |

### 整体流程

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="../assets/retry-decision-dark.svg">
  <img alt="错误重试判定流程：chatStream 调用 → err 是否为 nil → 放弃条件（取消/流已开始/满 4 次/不可重试）→ 放弃并回滚，或指数退避后重试" src="../assets/retry-decision-light.svg" width="890">
</picture>

## 一句话总结

> **非 200 分支**：把响应体（最多 512 字节）连同状态码塞进 `apiError` 当错误返回，状态码留给重试逻辑用。
> **`errors.As(err, &ae)`**：沿错误链逐层检查，找到 `*apiError` 并回填进 `ae`，把藏在 `error` 接口后面的具体字段掏出来。
> 配套记忆：`errors.As` 找**类型**，`errors.Is` 找**值**。
