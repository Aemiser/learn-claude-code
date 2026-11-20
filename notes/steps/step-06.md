# Step 6 前置导读 · Edit + Glob + Grep（写于代码之前）

> 里程碑 M2 步：完成后，一条自然语言指令可全自动「创建 → 编辑 → 检索」文件。

## 现在在做什么
补齐文件工具三件套：`Edit`（唯一匹配替换——本步主角）、`Glob`（按模式找文件名）、
`Grep`（按正则搜内容）。loop/model 依旧零改动，三个工具全部只是登记进 toolRegistry。

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="../assets/edit-flow-dark.svg">
  <img alt="Edit 执行流：防护闸（read-first/old==new/相对路径）→ 读文件 → 匹配计数三分支（0 处、多于 1 处、唯一匹配）→ 替换写回 → 带行号片段回传；失败分支均以 is_error 回填" src="../assets/edit-flow-light.svg" width="950">
</picture>

## 有什么用
- Edit：修改文件只发 diff（old→new），不用整文件重写——省 token 且把「改动意图」
  变成显式契约。S5 的行号格式在这里兑现价值：模型靠 Read 的行号地图写出唯一 old_string；
- Glob/Grep：agent 的「找」能力。没有它们，模型只能猜路径、靠 Read 硬翻。

## 架构位置
- L4.2b 内置工具（编辑与检索）
- 依赖：消费 S5 的行号格式（Edit 的 old_string 锚定）与 readFirst 状态（Edit 同样受防护）

## B 线证据（cc-haha 精读，均【社区证据（cc-haha）】）
| 事实 | 出处 |
|---|---|
| Edit 匹配 0 处 → "String to replace not found" | FileEditTool.ts:316-321 |
| 匹配 >1 且 !replace_all → "Found N matches... provide more context to uniquely identify" | FileEditTool.ts:331-336 |
| old_string == new_string → 拒绝 "No changes to make" | FileEditTool.ts:148-153 |
| 空 old_string + 文件不存在 → 创建新文件（合法） | FileEditTool.ts:225-226 |
| **Grep 底层是 ripgrep**（子进程调用） | GrepTool.ts:21 `import { ripGrep }` |
| "Prefer the Edit tool... it only sends the diff"（S5 已引） | FileWriteTool/prompt.ts |

## 关键设计决定
1. **唯一匹配是硬约束**：>1 匹配时拒绝执行，要求模型补上下文（多带相邻行）使
   old_string 唯一——这逼迫模型的每次改动**可预期、可验证**，杜绝「替换了错误的那处」；
2. **成功回传 ±3 行片段（带行号）**：让模型立刻自查改动落点是否正确（自验证闭环）；
3. **Glob 手写 `**` 双星遍历**（filepath.WalkDir）；**Grep 用 Go 标准库 regexp**——
   与真实 ripgrep 的差异是教学简化（ripgrep 快且默认尊 gitignore，我们都不做）；
4. Edit 同样过 read-first 闸（没读过的文件不许改——改比写更需要知道现状）。

## 刻意不做
路径越界防护（S8/L5）；Grep 的 -A/-B 上下文参数与 gitignore 尊重；Glob 的排序调优。

## 运行与验收（M2 剧本）
cd go; go run . 一条指令：
「在 D:\project\learnclaudecode\tmp-m2\ 下创建 notes.txt 写三行 abc/def/ghi，
然后把 def 那行改成大写，最后用 Grep 确认 ghi 还在」
预期：Write → Read（Edit 前）→ Edit → Grep 四连发全自动完成，最终回答确认。

## 与真实 CC 对照
工具行为面逐条对齐上表；官方工具五分类中「搜索」为一类【官方事实：how-claude-code-works】。
差异：Grep 非 ripgrep、Glob 无 ignore 语义（教学简化，已在导读立此存照）。
