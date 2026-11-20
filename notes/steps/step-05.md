# Step 5 前置导读 · Read + Write 工具（写于代码之前）

## 现在在做什么
给 agent 装上第一对「真本事」工具：`Read`（读文件，cat -n 行号 + 2000 行截断）与
`Write`（写文件，带 read-first 防护）。**loop.go 与 model.go 一行不改**——这正是
上一步 L4.1 注册表抽象要兑现的红利。

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="../assets/tool-registry-dark.svg">
  <img alt="Step 5 工具层架构：新增 Read/Write 登记进 toolRegistry，loop 与 model 零改动；Read 走 ReadFile→行号→截断，Write 走 WriteFile，二者落本地文件系统" src="../assets/tool-registry-light.svg" width="950">
</picture>

## 有什么用
没有文件工具的 agent 只会「说」；有了 Read/Write，它开始能「看你的代码、改你的世界」。
这也是第一次让模型的输出产生**持久副作用**——为 S8 权限层埋下真正的动机。

## 架构位置
- L4.2a 内置工具（文件读/写）
- L4.4 结果后处理：cat -n 行号（从 1 起）+ 默认 2000 行截断 + 截断提示
- L4.1 注册表扩容（新工具只登记，不改循环）

## B 线证据（cc-haha 精读，全部【社区证据（cc-haha）】）
| 事实 | 出处 |
|---|---|
| 工具名是 `Read`/`Write`（PascalCase），非 read_file | FileReadTool/prompt.ts `FILE_READ_TOOL_NAME='Read'` |
| 默认最多 2000 行 | FileReadTool/prompt.ts:10 `MAX_LINES_TO_READ = 2000` |
| cat -n 格式，行号从 1 起 | FileReadTool/prompt.ts:15 |
| file_path 必须绝对路径 | Read 描述 Usage 条款 |
| **写已存在文件前必须先 Read，否则失败**（read-first 防护） | FileWriteTool/prompt.ts |
| 修改优先用 Edit（只发 diff） | FileWriteTool/prompt.ts（S6 伏笔） |
| 文件未变时不重发内容（FILE_UNCHANGED_STUB） | FileReadTool/prompt.ts（本步不做） |

## 关键设计决定
1. **行号右对齐 6 位 + tab**（`fmt.Sprintf("%6d\t", n)`）——真实 cat -n 的格式，
   更重要的动机：S6 的 Edit 靠行号锚定 old_string；
2. **read-first 防护**：会话级 `readFirst map[string]bool` 记录读过哪些文件；Write 覆盖
   已存在文件前查表，没读过就报错。对应 cc-haha 的 FileStateCache 思想【社区证据】；
3. **截断而非报错**：超 2000 行读前 2000 行 + 末尾提示「还有多少行、用 offset 续读」——
   cc-haha limits.ts 的取舍结论同向（截断比抛错产出更有用）【社区证据】；
4. 相对路径直接报错（不猜 CWD）——错误信息是给模型的提示词。

## 刻意不做（诚实边界）
- **路径越界防护推迟到 S8（L5）**：此刻 Read/Write 可碰任意绝对路径——导读先立此存照；
- Edit 工具（S6）；FILE_UNCHANGED_STUB 缓存（不做，教学简化）；图片/PDF 读取（不做）。

## 运行与验收
cd go; go run .
1. 「读 D:\project\learnclaudecode\PLAN.md 并一句话总结」→ 🔧 Read → 终端可见行号化结果 → 模型总结；
2. 「在 D:\project\learnclaudecode\tmp-s5.txt 写入 hello」→ 文件出现；
3. 直接覆盖未读过的已存在文件 → 观察 read-first 报错回给模型（is_error）。

## 与真实 CC 对照
工具行为面逐条对齐上表 B 线证据；官方工具五分类把文件操作列为第一类【官方事实：
how-claude-code-works "Tools"】。我们的差异：无 file state 的 mtime 校验（教学简化）、
无图片多模态。
