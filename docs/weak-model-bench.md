# 弱模型基准（divvy-bench）

引擎的立论是"用工程约束而不是模型聪明程度把复杂目标做完"。mock 端到端测试证明流程能跑通，但不能回答唯一重要的问题：**换一个真实的弱模型，成功率、成本、失败形态如何？** `cmd/bench` 就是回答这个问题的可重复 harness。

## 快速开始

```bash
go run ./cmd/bench -list     # 语料库一览

# 本机 Ollama（默认端点）：
go run ./cmd/bench \
  -base-url http://127.0.0.1:11434/v1 \
  -model qwen3.8:27b \
  -extra '{"reasoning_effort":"none"}' \
  -root bench-runs/$(date +%Y%m%d-%H%M%S)

# 任何 OpenAI 兼容端点：
go run ./cmd/bench -base-url https://api.example.com/v1 -model small-model -api-key ...
```

常用参数：`-goals string-utils,fix-average` 只跑部分目标；`-repeat N` 全库重复 N 轮（弱模型输出有方差，单轮数字别当真）；`-timeout` 单目标墙钟预算（默认 20m）；`-max-steps/-max-retries/-parallel/-isolate/-git-commit` 透传引擎旋钮；`-out` 指定 JSON 报告路径（默认 `<root>/report.json`）。

## 语料库

固定、版本化在 `cmd/bench/corpus.go` 里——基准的目标集一旦漂移，跨运行的比较就失去意义。三个目标覆盖三种任务形态：

| 目标 | 形态 | 验收方式 |
|---|---|---|
| `string-utils` | 从零创建包（3 个函数 + 表驱动测试） | `go build` + `go test` |
| `fix-average` | 修复 seeded 差一 bug（含空切片 NaN） | 修复后 `go test` 通过 |
| `http-health` | 构建可运行 HTTP 服务 + 单测 | 构建 + 测试通过 |

选 Go 是因为它是唯一能**完全离线自验收**的栈（`go build`/`go test` 不需要网络），基准因此是 hermetic 的：结果差异只来自模型和引擎，不来自外部服务抖动。

`fix-average` 的 workspace 自带一个**会失败**的测试——harness 测试（`TestBench_SeedWorkspaceIsolomorphic`）守着这条不变量：如果语料本身不失败，这个目标就退化成了"从零创建"，基准失去区分度。它同时考察引擎最核心的弱模型能力：读失败输出 → 定位 → 最小修复。

## 采集的指标

每个目标一行：成败、墙钟、LLM 调用数、token、成本、叶子完成数，以及引擎自身的失败计数器（`verify_fail` / `retries` / `stalls` / `resplits`）。后者比成败更有信息量：一个"最终成功但重试 8 次"的模型和一个"一次过"的模型，在真实项目里的成本截然不同。

- token 取自任务树的全树累计（`TaskTree.TotalTokenUsage`），失败重试也计入——那正是基准要暴露的成本。
- 成本按内置价目表估算；本地模型（Ollama）不在表内，记为 $0，token 数仍然有效。
- 事件计数来自会话事件流（`agent.SummarizeEvents`），与 `-report` 复盘同一数据源。

## 工作区与事后解剖

每个目标一次运行一个目录：`<root>/<goal>-r<n>/`，内含 seed 的初始 git 提交、运行出的代码、以及该次运行的 `.divvy` 会话数据（树 JSON / 事件流 / 日志）。默认全部保留：

```bash
cd bench-runs/20261014-213000/fix-average-r1
git log --oneline            # 每个叶子一个提交（-git-commit 开启时）
git diff <seed>..HEAD        # 模型到底改了什么
divvy -report -session <id> -datadir .divvy   # 复用正式复盘
```

## 方法论注意事项

- **方差**：弱模型同题不同次的输出差异很大。结论请用 `-repeat 3` 以上的成功率区间，不要用单轮数字。
- **思考开关**：Qwen3 系默认开思考，叶子执行阶段推荐 `-extra '{"reasoning_effort":"none"}'`（约 3 倍 token 差，见 [qwen3.8-local.md](qwen3.8-local.md)）；拆解阶段保留思考质量更好。默认不传 extra，即引擎默认行为。
- **调参实验**：`-max-stall` 默认 3 对某些模型可能偏激进——这类"默认值是否合理"的问题正是基准要回答的：固定语料、只改一个旋钮、多轮重复。
- **harness 自身有测试**：`cmd/bench/bench_test.go` 用脚本化模型（`llm.ScriptedClient`）完整跑通一次目标运行，守住播种、git 基线、验收、指标采集全链路。harness 测试绿 = 基准失败可以归因于模型，不是工具坏了。

## 参考数字

来自 `docs/qwen3.8-local.md` 的一次手工运行（qwen3.8:27b @ Ollama，`reasoning_effort=none`，`-isolate -git-commit`）：「/health HTTP 服务 + 单测」任务，6 次 LLM 调用、5808 token、约 3.5 分钟完成，生成代码自带 200/405 两条测试。新跑法的报告 JSON 可与这组数字对照。
