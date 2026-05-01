# 中文冒烟 Workflow 优化循环

> 外层 harness 的可执行规范已经沉淀为 repo skill：`/Users/bytedance/symphony/.codex/skills/zh-smoke-harness/SKILL.md`。本文保留实验指标、issue 模板和历史记录。

## 目标

固定一个最小任务：在当前 issue worktree 根目录的 `SMOKE.md` 写入一行中文时间戳，然后本地验证、提交、AI Review、自动 merge 到当前实验分支。每轮只改变 workflow 或 orchestrator 的一个变量，用同类 issue 重跑，比较指标是否变好。

职责边界：

- `WORKFLOW.zh-smoke.md` 只描述单个 issue 内部怎么处理固定任务。
- `scripts/zh_smoke_round.py` 负责外层实验：创建同模板 issue、启动一次 Symphony、收集指标、追加实验记录。
- Codex/人类在每轮结束后读取实验记录，再决定下一轮只改一个变量。

## 固定任务

- 文件：`SMOKE.md`
- 内容：追加 `中文冒烟测试时间戳：<timestamp>`
- 验证：`rg -n '^中文冒烟测试时间戳：' SMOKE.md` 和 `git diff --check`
- 收口：本地 commit 后进入 Linear 状态 `AI Review`；通过后进入 `Merging`，本地 merge 成功后进入 `Done`；失败则进入 `Rework`
- 禁止：PR、push、Codex agent 写 Linear comment/status

## 指标

| 指标 | 来源 | 目标 |
| --- | --- | --- |
| `last_total_tokens` | `.symphony/logs/run-*.jsonl` 的 `thread/tokenUsage/updated.params.tokenUsage.last` | 判断单次上下文复杂度，越低越好 |
| `total_tokens` | `.symphony/logs/run-*.jsonl` 的 `thread/tokenUsage/updated.params.tokenUsage.total` | 多次 token event 的累计值，只作为总消耗参考 |
| `token_events` | `thread/tokenUsage/updated` 事件数量 | 越少越好，反映回合内事件密度 |
| `turn_duration_ms` | `.symphony/logs/run-*.jsonl` 的 `turn/completed` | 越低越好 |
| `total_flow_ms` | `Todo -> In Progress` 到 `Merging -> Done` | 端到端耗时，越低越好 |
| `dispatch_latency_ms` | `Todo -> In Progress` 到 `turn_started` | 调度耗时，越低越好 |
| `post_turn_handoff_ms` | `turn_completed` 到进入 review 状态 | 框架交接耗时，越低越好 |
| `command_events` / `plan_updates` / `diff_updates` | Codex event 流 | agent 活动密度，越低通常越快 |
| `success` | `Done` 或 `Human Review` 且有 commit | 必须为 `true` |
| `ai_review_result` | `.symphony/logs/run-*.jsonl` 的 `ai_review_completed` / `ai_review_failed` | 应为 `passed` |
| `merge_duration_ms` | `.symphony/logs/run-*.jsonl` 的 `local_merge_completed` | 越低越好 |
| `changed_files` | `In Progress -> AI Review` 的 `changed_files` 字段 | 应该只有 `SMOKE.md` |
| `workpad_updates` / `workpad_phases` | orchestrator 的 `workpad_updated` 结构化日志 | Issue 评论动作应可追踪，阶段清楚 |
| TUI 可读性 | 人工打分 1-5，必要时截图/帧文本 | 有语义颜色、无原始 JSON、不误入 Backoff，当前状态一眼可读 |
| 结构化与审美 | 人工打分 1-5 | Workpad/TUI/实验记录短、分区清楚、有行动状态 |

## 通用性约束

- 固定中文冒烟任务只是探针，用来暴露框架问题，不把 `SMOKE.md`、固定命令或固定 issue 写死进 orchestrator。
- 框架优化优先落在通用 primitive：状态机、worktree 生命周期、Linear workpad、日志指标、TUI、retry、调度、workflow 可声明 hook/check。
- 对 agent 自己执行的 `git status`、`git diff`、验证命令，优先统计动作密度和耗时；只有抽象成通用 workflow hook/check 时，才考虑由框架执行。
- 评测不仅看 token/耗时，也看 TUI 可读性、Issue comment 动作记录、结构化文本质量和人类可审阅性。

## 棘轮规则

每轮只做一个优化，参考 darwin-skill 的 `baseline -> improve -> validate -> keep/revert`：

1. 记录基线：

   ```bash
   make zh-smoke-metrics LOG=.symphony/logs/<run>.jsonl ISSUE=<ZEE-xx>
   ```

2. 只改一个变量，例如 prompt 长度、reasoning effort、poll interval、TUI 文案、自动评审策略或 retry 策略。
3. 结束旧的 TUI 进程，重启专用 TUI，用同样的中文冒烟 issue 模板再跑一轮。
4. 再记录指标，并检查 `changed_files` 是否仍然只有 `SMOKE.md`。
5. 只有满足下面条件才保留改动：
   - `success=true`
   - `changed_files=SMOKE.md`
   - `last_total_tokens`、`token_events`、`turn_duration_ms`、`total_flow_ms`、`merge_duration_ms` 或 TUI/comment 可读性至少一项变好
   - 没有新增 Backoff 噪声或错误状态
6. 如果指标退步且没有明确收益，进入 Rework 优化：只针对退步项做一个最小调整，然后重新跑同样任务。

## 自动阶段

这个专用 workflow 开启 `agent.ai_review.enabled: true`，并使用真实 Linear 状态 `AI Review`，但不改变 `Human Review` 的人工语义：

1. `Todo -> In Progress`：创建 worktree，写 `SMOKE.md`，验证并提交。
2. `In Progress -> AI Review`：orchestrator 检测到本地 commit 后写中文 Workpad，并把 issue 移动到 `AI Review`。
3. `AI Review -> Merging`：评审通过且 `auto_merge=true` 时自动推进。
4. `AI Review -> Rework`：评审失败且 `rework_on_failure=true` 时自动返工。
5. `Merging -> Done`：orchestrator 本地 merge `symphony-go/<issue>` 到当前实验分支，记录 merge 耗时。
6. 如果指标显示需要优化，下一轮将改动控制在 workflow/orchestrator 的一个变量上；Linear 写操作仍由 orchestrator/tooling 层负责，不由 Codex child process 执行。

## 推荐命令

先确认当前没有旧的专用 TUI：

```bash
pgrep -af 'symphony-go run --workflow ./WORKFLOW.zh-smoke.md'
```

启动专用 TUI：

```bash
make zh-smoke-run
```

只处理一个 issue，适合做可重复 benchmark：

```bash
make zh-smoke-once ISSUE=ZEE-16
```

采集本轮指标并追加到结果表：

```bash
make zh-smoke-metrics ISSUE=ZEE-16
```

结果默认写入 skill 实验目录：

```text
../.codex/skills/zh-smoke-harness/experiments/results.tsv
../.codex/skills/zh-smoke-harness/experiments/rounds.md
```

本文只保留历史说明；新的实验记录不要再追加到本文。

创建同模板 issue、跑一次、采集指标：

```bash
make zh-smoke-round
```

默认 merge target 是当前 git 分支。如果想指定其他 merge target：

```bash
make zh-smoke-round MERGE_TARGET=<branch>
```

只验证 Linear project/team/state 解析，不创建 issue：

```bash
python3 scripts/zh_smoke_round.py --dry-run
```

## 三轮闭环

每一轮都使用同一个 issue 模板，只改变 Linear issue 编号和本地时间戳。建议在实验分支内运行，因为 `AI Review -> Merging` 会真实 merge 到当前 merge target。

### Round 1：基线

1. 创建同模板 issue，并直接放到 `Todo`。
2. 跑一次专用 workflow：

   ```bash
   make zh-smoke-round
   ```

3. 等待流程走到 `Done`，脚本会把指标追加到 TSV 和本文 `实验记录`。

### Round 2：一次最小优化

1. 根据 Round 1 结果只改一个变量，例如：
   - 缩短 prompt。
   - 调整 `model_reasoning_effort`。
   - 调整 `polling.interval_ms`。
   - 改善 TUI 展示文案。
2. 停掉旧进程：

   ```bash
   make zh-smoke-stop
   ```

3. 重新编译并启动：

   ```bash
   make build
   make zh-smoke-run
   ```

4. 创建同模板的新 issue，仍直接进入 `Todo` 并跑完：

   ```bash
   make zh-smoke-round
   ```

5. 对比 skill 实验目录下的 `results.tsv` 和 `rounds.md`。如果 `success=true` 且 token、耗时、TUI 可读性或 comment 可读性更好，保留该优化；否则按 Rework 思路回退或改一个更小变量。

### Round 3：棘轮确认

1. 基于 Round 2 的更优版本再做一个最小优化。
2. 重复停止旧进程、重编译、重启、创建同模板 issue、跑完、采集指标。
3. 三轮结束后对比：

   ```bash
   tail -n 4 ../.codex/skills/zh-smoke-harness/experiments/results.tsv
   ```

4. 只保留能稳定让指标变好的改动。

## Issue 模板

```md
中文冒烟测试：固定时间戳任务

请使用中文冒烟 benchmark workflow 处理：

- 只改 `SMOKE.md`
- 写入一行 `中文冒烟测试时间戳：<timestamp>`
- 运行 `rg -n '^中文冒烟测试时间戳：' SMOKE.md`
- 运行 `git diff --check`
- 创建一个本地 commit
- 不创建 PR，不 push
- 所有 Workpad、状态说明和最终回复使用中文
```

## 实验记录

| 时间 | Issue | 状态 | 成功 | Token | Events | Flow ms | Turn ms | AI Review | Merge ms | Workpad | 文件 | 日志 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 2026-05-01T19:58:08 | ZEE-17 | Done | true | 241220 (last 35274, events 7) | tok 7, cmd 11, plan 0, diff 6 | 64153 | 48514 | passed | 117 | 0 | SMOKE.md | `.symphony/logs/run-20260501-195659.jsonl` |
| 2026-05-01T20:00:52 | ZEE-18 | Done | true | 491239 (last 36427, events 14) | tok 14, cmd 13, plan 5, diff 10 | 87161 | 71452 | passed | 90 | 0 | SMOKE.md | `.symphony/logs/run-20260501-195920.jsonl` |
| 2026-05-01T20:07:03 | ZEE-19 | Done | true | 410279 (last 35293, events 12) | tok 12, cmd 10, plan 5, diff 9 | 88175 | 74399 | passed | 95 | 0 | SMOKE.md | `.symphony/logs/run-20260501-200530.jsonl` |

## 三轮初步诊断

| Issue | Flow ms | Dispatch ms | Handoff ms | AI Review ms | Merge state ms | Command events | Plan updates | Diff updates |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| ZEE-17 | 64153 | 6602 | 4579 | 1421 | 3036 | 11 |  | 6 |
| ZEE-18 | 87161 | 5837 | 4823 | 1545 | 3502 | 13 | 5 | 10 |
| ZEE-19 | 88175 | 4931 | 4622 | 1391 | 2832 | 10 | 5 | 9 |

初步结论：ZEE-18/ZEE-19 的 merge 和 AI Review 都不慢，耗时主要增加在 Codex turn 内部。`last_total_tokens` 三轮接近，但 `token_events`、`plan_updates`、`diff_updates` 有明显差异，说明下一轮应优先观察和优化 agent 回合内活动密度，而不是做 task-specific fast path。
