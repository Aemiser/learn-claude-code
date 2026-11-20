# Step 7 前置导读 · Bash 工具 + 并发工具调用（写于代码之前）

> 本步后，agent 获得**执行命令**的能力，并能在一条 assistant 消息里**并行调用多个工具**。
> 覆盖两层：L4.3（执行类内置工具 Bash）与 L3.5（并发 tool_use）。

## 上一步思考题答案（R19 闭环）

- S6-1：Edit 的「>1 匹配就拒绝」是安全关键，因为 >1 匹配时 old_string 有歧义——模型自以为改 A 处，
  实际可能改到长得一样的 B 处，且**改错而不自知**（最危险的一类事故）。拒绝迫使它补上下文使 old_string 唯一，
  把「改哪里」变成可验证契约。实测模型甚至预判会失败仍执行——约束已内化进其决策。
- S6-2：真 ripgrep 默认尊重 .gitignore，跳过大仓库里 node_modules/、dist/、.git 等；我们的全量遍历会把这些
  噪声全搜出来，模型可能把依赖内部匹配当成项目代码，在错误的地方读/改、下错误结论。

## 现在在做什么
在 toolRegistry 里新增 `Bash` 工具（L4.3），并把 loop 的「回填」段改成并发执行（L3.5）。
Bash 把命令交给 shell 子进程，回传 stdout+stderr 与 exit code；concurrency 让同一消息里的多个 tool_use 并行跑。

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="../assets/step7-bash-concurrency-dark.svg">
  <img alt="Step 7 数据流：L3 Agent Loop 收到含 N 个 tool_use 的模型响应 → 解析分发 → L4.3 并行启动 Bash 子进程 → 各自回传 stdout/stderr → 截断（30000 字符上限）→ 合流成一条 user 消息回填 tool_result" src="../assets/step7-bash-concurrency-light.svg" width="960">
</picture>

## 有什么用
- **Bash**：agent 从「只能读/改/搜文件」升级为「能跑命令」——跑测试、git、构建、任何 CLI，终端编程 agent 的核心自由度来源。
- **并发**：当一句话里有多个相互独立的工具调用（如查两个不同文件、跑两条不相关的命令），并行使总耗时≈最慢的那一个，
  而不是累加。真实 agent 大量靠这个提效。

## 架构位置
- L4.3 内置工具（执行类）；消费 L4.4 的结果截断（Bash 输出超 30000 字符截断）
- L3.5 并发工具调用：在 L3.3 回填段内，把「串行 for」改成 goroutine + WaitGroup
- 依赖：L4.1 注册表 + L4.4 截断；Bash 工具经 `init()` 登记，model.go 的 `toolSpecs()` **零改动**自动带出

## B 线证据（cc-haha 精读，均【社区证据（cc-haha）】）
| 事实 | 出处 |
|---|---|
| 工具名 `Bash`（与 PowerShellTool 分立成两个工具，而非一个工具带 shell 参数） | tools/BashTool/toolName.ts `BASH_TOOL_NAME='Bash'` |
| 默认超时 120_000ms、上限 600_000ms | utils/timeouts.ts:2-3 `DEFAULT_TIMEOUT_MS / MAX_TIMEOUT_MS` |
| 输出上限 30_000 字符（默认） | utils/shell/outputLimits.ts `BASH_MAX_OUTPUT_DEFAULT` |
| stderr 合并进 stdout | BashTool.tsx 多处 "stderr is merged into stdout" |
| exit code 语义：默认「非 0 即失败」；grep/rg/find/diff 有特例语义 | tools/BashTool/commandSemantics.ts `DEFAULT_SEMANTIC` + `COMMAND_SEMANTICS` |
| 描述文案「The working directory persists…shell state does not」+ 并发指令「make multiple bash calls in a single message」 | tools/BashTool/prompt.ts `getSimplePrompt()` |

## 关键设计决定
1. **stderr 合并进 stdout、exit code 单独前置**：模型既能看到报错内容，又能拿 exit code 判定成功/失败（前者给人看，后者给机器判定）。
2. **exit code 非 0 → 返回 error（is_error=true）**：对齐 cc-haha DEFAULT_SEMANTIC。grep/find/diff 的特例语义本步**刻意不做**，留作后续。
3. **工作目录跨命令持续，shell 环境不持续**：用一个包级 `bashCwd` 手动追踪「纯 `cd <path>`」，每次子进程从该目录启动；env、变量等由 shell 在子进程内自生自灭。
4. **并发 = goroutine + WaitGroup**：同一轮的所有 tool_use 并行执行，`wg.Wait()` 是合流点——全部 tool_result 就绪后才作为**一条** user 消息回填。

## 刻意不做
- 危险命令识别 / 权限确认（S8/L5——Bash 现在能碰任何东西但没人把关，这是 S7 之后必须立刻补的最大空壳）
- PowerShellTool 分立、后台任务（run_in_background）、超时不杀子进程的精细处理（CommandContext 已杀）
- grep/find/diff 的 exit code 特例语义表、cd 复合命令追踪（`cd x && y`）、输出中途流式（先跑完再回传）

## 运行与验收
```
cd go && go run .
```
1. 单条 Bash：「请用 Bash 执行 echo step7-ok，并告诉我返回什么」→ 🔧 调用 + ✓ 返回 `step7-ok`。
2. 工作目录持续：「第一步 cd ..（仅这仨字符），第二步 pwd（仅这俩字符），都单独 Bash 调用，返回第二步」
   → 第二步返回上级目录（追踪生效）。
3. 并发（时间/顺序签名）：请求两独立 Bash 调用 → 终端先打印两个 🔧，之后才两个 ✓（串行会是 🔧✓🔧✓）。

## 与真实 CC 对照
Bash 工具名、超时默认/上限、输出 30000 字符截断、stderr 合并、exit code 判定均对齐上表【社区证据】；
并发「一条消息多 tool_use 并行」来自 BashTool/prompt.ts 的显式引导【社区证据】。
简化：单 shell 不建 PowerShellTool、无权限层（S8 补）、无后台执行、exit code 无特例表。