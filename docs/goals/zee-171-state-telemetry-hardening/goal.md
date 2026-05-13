# ZEE-171 State and Telemetry Hardening

## Objective

收紧 ZEE-171 暴露出的 issueflow 状态所有权、continuation turn 边界、SigNoz 观测验收口径和单 issue 可见日志过滤，直到代码、文档、测试、构建和 smoke 证据共同证明 `Todo -> In Progress -> AI Review -> Pushing -> Done` 链路按新契约运行。

## Original Request

基于 ZEE-171 的 smoke 发现，收紧 `AI Review -> Pushing -> Done` 状态所有权，强制 `Review: PASS` 与 `Push: PASS` 分轮处理，修正 metrics 不按 `issue_identifier` 查询，统一 SigNoz logs 为生命周期日志事实源，并让 `--issue <ISSUE>` 控制台只输出当前 issue。

## Intake Summary

- Input shape: `existing_plan`
- Audience: Symphony Go workflow/operator and future AI agents running issueflow smoke
- Authority: `requested`
- Proof type: `test`
- Completion proof: 相关单测通过、`./build.sh` 通过、目标 smoke issue 观察到 `Todo -> In Progress -> AI Review -> Pushing -> Done`，SigNoz traces/logs/metrics 按新口径可查，console noise 检查不出现非目标 issue。
- Likely misfire: 只改文档或只让状态最终到 `Done`，但仍允许 `AI Review` turn 内同轮 push、由 agent 越权推进状态，或继续把 metrics issue 级查询当验收。
- Blind spots considered: 现有 `Merging` 旧语义可能仍残留；OTel logs 和本地 `.human.log/.jsonl` 的事实源边界容易混淆；`--issue` 过滤如果晚于 preflight/output 会继续泄漏旧 issue；真实 SigNoz smoke 依赖本机 OTLP collector 和可用 issue。
- Existing plan facts: 用户给定了任务 1-5、代码锚点、验证命令、SigNoz SQL 和 console noise 验收点，后续 `/goal` 必须保留并逐项验证，不得把 metrics `issue_identifier` 查询失败当作失败。

## Goal Kind

`existing_plan`

## Current Tranche

当前 tranche 是完整交付这次 ZEE-171 后续收紧，而不是只做计划或单文件修补。`/goal` 应先校验用户给定计划与当前代码事实，再按最小影响拆成连续安全 Worker slice：状态所有权与 continuation、telemetry/logs 与文档口径、单 issue 可见日志过滤、最终验证与 smoke 证据归档。每个 slice 完成后立即审计并推进下一张必要卡，直到最终审计证明完整 outcome 达成。

## Non-Negotiable Constraints

- 默认中文输出，结论先行，带 `file:line`、命令输出或日志片段证据。
- 遵守本仓 `AGENTS.md`：非平凡任务先规划，最小影响，根因导向，完成前验证。
- `$goal-prep` 只创建 GoalBuddy 控制文件；实现留给后续 `/goal`。
- 不重引入旧 `Merging` 语义到新 `Pushing` 契约中。
- `applyReviewPass` 和 `applyPushPass` 必须保持框架状态写入口；agent 只能通过受限文本信号触发框架动作。
- 如果当前 turn 起始状态是 `AI Review`，即使输出里同时包含 push 结果，也只能进入 `Pushing` 并发起 `PushingContinuationPromptText`。
- 只有起始状态已是 `Pushing` 的 turn 才可识别 `Push: PASS` 并进入 `Done`。
- Metrics 验收不按 `issue_identifier`；issue drilldown 只走 traces/logs。
- SigNoz logs 是 lifecycle logs 主验收事实源；本地 `.human.log/.jsonl` 只作 fallback/debug。
- `--issue <ISSUE>` 可见 console/human output 不得打印非目标 issue 的 section、workspace retained 或 dispatch 相关日志。
- 修改 Go 行为后优先运行 `./test.sh ./internal/service/issueflow ./internal/service/orchestrator ./internal/runtime/telemetry`，再运行 `./build.sh`。

## Stop Rule

Stop only when a final audit proves the full original outcome is complete.

Do not stop after planning, discovery, or Judge selection if a safe Worker task can be activated.

Do not stop after a single verified Worker slice while required slices for status ownership, telemetry/logs, docs metrics口径, console filtering, and final validation remain queued or safely actionable.

If smoke needs an external issue, Linear credentials, OTLP collector, or SigNoz access that is unavailable, block only the smoke task with a receipt, complete all local non-destructive verification, and leave an exact operator handoff.

## Canonical Board

Machine truth lives at:

`docs/goals/zee-171-state-telemetry-hardening/state.yaml`

If this charter and `state.yaml` disagree, `state.yaml` wins for task status, active task, receipts, verification freshness, and completion truth.

## Run Command

```text
/goal Follow docs/goals/zee-171-state-telemetry-hardening/goal.md.
```

## PM Loop

On every `/goal` continuation:

1. Read this charter.
2. Read `state.yaml`.
3. Run the bundled GoalBuddy update checker when available and mention a newer version without blocking.
4. Re-check the intake: original request, input shape, authority, proof, blind spots, existing plan facts, and likely misfire.
5. Work only on the active board task.
6. Assign Scout, Judge, Worker, or PM according to the task.
7. Write a compact task receipt.
8. Update the board.
9. If Judge selected a safe Worker task with `allowed_files`, `verify`, and `stop_if`, activate it and continue unless blocked.
10. Treat a slice audit as a checkpoint, not completion, unless it explicitly proves the full original outcome is complete.
11. Finish only with a Judge/PM audit receipt that maps receipts and verification back to the original user outcome and records `full_outcome_complete: true`.
