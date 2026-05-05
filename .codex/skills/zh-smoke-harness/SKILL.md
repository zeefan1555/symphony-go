---
name: zh-smoke-harness
description: "外层中文冒烟实验 harness：控制 Symphony Go 创建固定 Linear issue、运行 zh-smoke workflow、采集中间指标、判断优化点、修改代码并重启验证。Use when user asks to run/optimize zh-smoke, Symphony Go smoke workflow, AI Review metrics, 三轮闭环, token/speed/TUI optimization."
---

# Zh Smoke Harness

这个 skill 是外层 harness，不是单个 Linear issue 的工作流。它负责控制框架本身：创建相同中文冒烟 issue、启动 Symphony Go、收集指标、判断下一处最小优化、修改代码、重启后再跑。

单个 issue 的任务定义在 `WORKFLOW.zh-smoke.md`；本 skill 只编排实验循环。

## 成功标准

- 每轮都创建同模板 Linear issue，并从 `Todo` 开始。
- 每轮都走固定状态流：`Todo -> In Progress -> AI Review -> Merging -> Done`，失败则进入 `Rework`。
- 每轮都把指标追加到：
  - `.codex/skills/zh-smoke-harness/experiments/results.tsv`
  - `.codex/skills/zh-smoke-harness/experiments/rounds.md`
- 每轮只改一个优化变量，避免无法归因。
- 每轮代码改造验证通过后都创建一个本地 commit，commit message 写清楚本轮新增的框架能力。
- 只有指标变好且没有新增异常状态，才保留代码改动。

## 固定资产

- Workflow：`WORKFLOW.zh-smoke.md`
- 外层 round 脚本：`scripts/zh_smoke_round.py`
- 指标脚本：`scripts/smoke_metrics.py`
- 人类可读实验记录：`.codex/skills/zh-smoke-harness/experiments/rounds.md`
- 机器可读 TSV：`.codex/skills/zh-smoke-harness/experiments/results.tsv`
- 默认 merge target：当前 git 分支；推荐实验分支 `zh-smoke-harness-loop`

## 指标

| 指标 | 判定 |
| --- | --- |
| `success` | 必须为 `true` |
| `changed_files` | 应该只有 `docs/smoke/pr-merge-fast-path-smoke.md` |
| `ai_review_result` | 应为 `passed` |
| `last_total_tokens` | 单次上下文 token，越低越好；优先用它判断单轮复杂度 |
| `total_tokens` | Codex 多次 token event 的累计值，只用于估算总消耗，不单独作为优化成败依据 |
| `token_events` | 越少越好；反映 agent 回合内思考/工具/消息节奏 |
| `turn_duration_ms` | 越低越好 |
| `total_flow_ms` | 从 `Todo -> In Progress` 到 `Merging -> Done` 的端到端耗时 |
| `dispatch_latency_ms` | 状态进入 `In Progress` 到 Codex turn 开始的调度耗时 |
| `post_turn_handoff_ms` | Codex turn 完成到进入 review 状态的交接耗时 |
| `merge_duration_ms` | 越低越好 |
| `command_events` / `plan_updates` / `diff_updates` | agent 活动密度；越少通常越快，但不能牺牲必要验证 |
| `workpad_updates` / `workpad_phases` | Issue comment 动作记录；应能看出 initial、handoff、review、merge 等阶段 |
| TUI 可读性 | 无 raw JSON、无误导 Backoff、颜色/分区/状态文案一眼可读 |
| 结构化与审美 | Workpad、最终记录、TUI 文案应短、分区清楚、有行动状态，不堆原始日志 |

## 实验记录格式

每轮结束后都要更新 `.codex/skills/zh-smoke-harness/experiments/rounds.md`，让人能直接 review：

- **本轮目标**：这轮优化了哪个通用框架能力。
- **代码改动**：列出 2-4 条关键改动和对应文件。
- **指标表**：至少包含 success、final_state、changed_files、AI Review、last_total_tokens、total_tokens、token_events、turn_duration_ms、total_flow_ms、dispatch_latency_ms、post_turn_handoff_ms、ai_review_ms、merge_state_ms、command_events、plan_updates、diff_updates、workpad_updates、workpad_phases、TUI/readability 观察。
- **优化判断**：哪些维度变好、哪些退步、是否符合预期。
- **证据**：写明 issue、log、commit、验证命令。
- **下一轮候选**：只列最值得尝试的一个通用优化点。

`results.tsv` 用于脚本对比，`rounds.md` 用于人工 review。不要再把新实验记录追加到 `go/docs/plan/zh-smoke-workflow-optimization.md`。

运行 live round 时要用 `CHANGE_NOTE` 写清本轮优化动作：

```bash
make zh-smoke-round CHANGE_NOTE="增加 workpad 更新结构化指标"
```

如果只是回填历史指标，可用：

```bash
make zh-smoke-metrics ISSUE=<ZEE-xx> LOG=<run-log> CHANGE_NOTE="回填历史指标"
```

## 通用性原则

- 只优化框架 primitive：调度、日志、指标、TUI、workpad、状态机、retry、prompt 裁剪。
- 不为固定 smoke 文件、固定 issue、固定命令写死 fast path。
- 固定 smoke 任务只是探针，用来暴露通用框架问题；优化必须能解释为对任意代码任务也有价值。
- 对 agent 的 `git status`、`git diff`、验证命令等动作，优先做观测和统计，不把这些任务步骤搬进 orchestrator。
- 如果要新增自动化执行能力，必须抽象成 workflow 可声明的通用 hook/check，而不是写死在 zh-smoke harness。

## Phase 0：Preflight

1. 进入 Go 模块：

   ```bash
   cd /Users/bytedance/symphony-go
   ```

2. 查看工作区：

   ```bash
   git status --short
   ```

3. 确认当前分支：

   ```bash
   git branch --show-current
   ```

4. 推荐在专用实验分支运行：

   ```bash
   git switch zh-smoke-harness-loop
   ```

   如果分支不存在，再创建：

   ```bash
   git switch -c zh-smoke-harness-loop
   ```

5. 如果当前有未提交的框架改动，先说明风险：`AI Review -> Merging` 会真实 merge 到 `MERGE_TARGET`，默认就是当前分支。只有在用户确认、或显式设置临时 `MERGE_TARGET=<branch>` 时，才跑 live round。
6. 验证 Linear 解析，不创建 issue：

   ```bash
   python3 scripts/zh_smoke_round.py --dry-run
   ```

7. 确认脚本和构建可用：

   ```bash
   python3 -m py_compile scripts/smoke_metrics.py scripts/zh_smoke_round.py
   go test ./...
   make build
   git diff --check
   ```

## Phase 1：Baseline Round

运行一轮固定冒烟：

```bash
make zh-smoke-round
```

该命令会：

1. 创建同模板 Linear issue。
2. 直接设为 `Todo`。
3. 停止旧 zh-smoke 进程。
4. 记录 `.symphony/logs/run-*.jsonl` 快照。
5. 运行一次 `WORKFLOW.zh-smoke.md`，每轮都启动新的 Symphony Go 进程。
6. 只用本轮新生成的 log 追加 TSV 和 Markdown 指标记录。
7. 如果设置了 `CHANGE_NOTE`，把它写入 `rounds.md` 的“优化动作”列。

如果不能真实运行，至少用现有 `.symphony/logs/run-*.jsonl` 做 dry-run 指标解析，并在结论中标注不是 live round。

## Phase 2：诊断

读取最近几轮结果：

```bash
tail -n 4 ../.codex/skills/zh-smoke-harness/experiments/results.tsv
tail -n 40 ../.codex/skills/zh-smoke-harness/experiments/rounds.md
```

按优先级判断问题：

1. `success=false`：先修状态流、merge、AI Review 或工作区问题。
2. `changed_files` 不是 `docs/smoke/pr-merge-fast-path-smoke.md`：先收紧 workflow prompt 或 AI Review 规则。
3. `ai_review_result=failed`：先看失败原因，决定是否改 review 规则或 issue prompt。
4. `last_total_tokens` 高：优先缩短通用 prompt、减少无关上下文、降低 reasoning effort。
5. `total_tokens` 高但 `last_total_tokens` 稳定：先看 `token_events`，通常是回合内事件次数变多。
6. `turn_duration_ms` 高：优先看 `token_events`、`command_events`、`plan_updates`、`diff_updates`，确认慢在 agent 内部还是框架交接。
7. `dispatch_latency_ms`、`post_turn_handoff_ms`、`ai_review_ms`、`merge_state_ms` 高：优先看 orchestrator 调度、Linear 写入和本地 merge。
8. `workpad_updates` 为空或阶段不清楚：先补 orchestrator 结构化日志，不让 child agent 自己写 Linear comment。
9. TUI 不清楚：优先改 `internal/tui` 展示，不改业务状态流。

## Phase 3：一次最小优化

每轮只能改一个变量。常见候选：

- 缩短 `WORKFLOW.zh-smoke.md` 的说明文本。
- 调整 `model_reasoning_effort`。
- 调整 `polling.interval_ms`。
- 改善 `internal/tui` 的状态或指标展示。
- 调整 `internal/orchestrator` 的 AI Review 判定。
- 改善 `scripts/smoke_metrics.py` 的字段提取。
- 增加通用事件指标，例如 workpad 更新、状态流耗时、agent 活动密度。

修改后必须验证：

```bash
python3 -m py_compile scripts/smoke_metrics.py scripts/zh_smoke_round.py
go test ./...
make build
git diff --check
```

验证通过后必须提交本轮代码改造：

```bash
git status --short
git add <本轮相关文件>
git commit -F /tmp/zh-smoke-commit-message.txt
```

提交要求：

- commit subject 使用 `feat(...)`、`fix(...)` 或 `chore(...)`，直接说明新增能力。
- body 写清 `Summary`、`Rationale`、`Tests`。
- 只提交本轮框架/skill/实验记录相关文件，不提交无关 worktree、临时文件或外部目录。
- 每个 commit 对应一个可解释的优化点，方便后续按指标回滚或保留。

## Phase 4：重启并复测

停止旧进程、重新跑一轮：

```bash
make zh-smoke-stop
make build
make zh-smoke-round
```

`make zh-smoke-round` 内部也会再次停止旧进程，并拒绝复用旧日志。

然后对比最近两轮：

```bash
tail -n 3 ../.codex/skills/zh-smoke-harness/experiments/results.tsv
```

保留条件：

- `success=true`
- `changed_files=docs/smoke/pr-merge-fast-path-smoke.md`
- `ai_review_result=passed`
- 至少一个主要指标变好：`total_tokens`、`turn_duration_ms`、`merge_duration_ms` 或 TUI 可读性
- 没有新增 Backoff、错误状态或额外文件改动

如果退步：

- 说明退步指标。
- 不做第二个无关优化。
- 回滚本轮改动，或只针对退步项进入下一轮 Rework。

## Phase 5：三轮循环

默认先跑三轮：

1. Round 1：Baseline，不改代码，只记录。
2. Round 2：针对最明显瓶颈做一个最小优化，重启并复测。
3. Round 3：如果 Round 2 变好，再做一个更小的确认优化；否则回滚并尝试另一个单变量。

三轮结束输出：

- 三轮 issue id。
- 每轮 `total_tokens / turn_duration_ms / ai_review_result / changed_files / final_state`。
- 每轮对应的本地 commit hash 和功能说明。
- 保留了哪些改动，为什么。
- 没保留哪些改动，为什么。
- 下一轮最值得尝试的一个变量。

## 边界

- 不让 issue 内的 Codex agent 创建新 issue、改 Linear 状态或写指标文档。
- Linear 写操作只允许外层 harness 或 Go orchestrator 执行。
- 不在脏的主 checkout 上强行跑 `AI Review -> Merging`，除非用户明确接受真实 merge 风险。
- 不一次性做多项优化。
- 不为了降 token 删除必要验证。

## Darwin 对齐

本 skill 采用 Darwin skill 的实验范式：

- 单一目标：固定中文冒烟任务的框架效率。
- 双重评估：静态代码/流程检查 + live round 指标。
- 棘轮机制：只有指标变好才保留。
- 人在回路：每轮结束说明证据和风险，由用户决定是否继续扩大自动化。
- 可回滚：每次只改一个变量，便于 revert。
