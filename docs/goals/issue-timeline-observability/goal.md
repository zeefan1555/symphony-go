# Issue Timeline Observability

## Objective

把单个 issue 的观测体验做成完整 Timeline：按 `issue_identifier=<ISSUE>` 查询 Trace/Logs 时能看到完整 run、状态流转、Codex turn、hook/prompt/command/file-change/final 摘要等生命周期事件及耗时；Metrics 继续保持低基数，只服务全局聚合。

## Original Request

用户提供了 “Issue Timeline Observability 优化计划”，要求修正 `issue_run` trace span 生命周期、记录 Codex turn 真实耗时、让 turn timeline 字段按 continuation 当前状态计算、扩充 OTel logs 精选事件、保持 metrics 不引入 `issue_identifier` 等高基数字段，并更新 SigNoz 文档和测试/smoke 验收。

## Intake Summary

- Input shape: `existing_plan`
- Audience: Symphony Go 的本地/远端 issue runner 使用者，以及用 SigNoz 查询单 issue timeline 的开发者。
- Authority: `requested`
- Proof type: `test`
- Completion proof: 单测、局部验证、构建、`git diff --check` 均通过；真实 issue smoke 能在 SigNoz traces/logs 中按 `issue_identifier=<ISSUE>` 查到完整 timeline；metrics 查询不依赖高基数字段且仍能按全局 label 聚合。
- Likely misfire: 只补了一堆日志或 instant spans，看起来事件更多，但 `issue_run` 和 `codex_turn_completed` 仍没有真实耗时，或把 `issue_identifier/session_id/turn_id` 错误塞进 metrics label 造成高基数。
- Blind spots considered: 当前仓库已有 telemetry 结构和 issueflow 状态语义需要先核实；真实 SigNoz smoke 可能依赖本地 collector/凭据/真实 Linear issue；`feat_zff` 上既有 workflow reasoning effort 改动必须保留；精选 logs 需要避免泄漏原始 event stream、message delta 和长 shell output。
- Existing plan facts:
  - `issue_run` span 应覆盖 `RunIssueTrunk` 完整 run，而不是创建后立即结束。
  - Codex turn 需要记录 `started_at`、`completed_at`、`duration_ms`，并扩展 `codex.Result` 供 `AfterTurn` 使用。
  - `AfterTurn` 要按 `turnStartIssue.State` 计算 continuation 当前 turn 的 `phase` / `stage`。
  - turn span/log 需要 `turn_count`、`continuation`、`phase`、`stage`、`state`、`duration_ms`、`session_id`、`turn_id`。
  - OTel logs 新增精选事件 `codex_turn_started`、`codex_final`、`codex_command`、`codex_file_change`，但不导出 raw payload、长 output、token delta 或完整 Codex event stream。
  - Metrics 不允许 `issue_id`、`issue_identifier`、`session_id`、`turn_id` 进入 labels。
  - 文档要补 SigNoz Issue Timeline drilldown 查询示例。

## Goal Kind

`existing_plan`

## Current Tranche

当前 tranche 是把用户提供的计划转成可验证的实现闭环：先只读核实当前代码结构和计划边界，再由 Judge 收敛成最小安全 Worker 切片，随后连续完成实现、测试、文档和 smoke 证据，直到 final audit 能把所有 receipts 映射回原始目标。

## Non-Negotiable Constraints

- 不把 `issue_identifier`、`issue_id`、`session_id`、`turn_id` 放进 metrics labels。
- 不导出完整 Codex event stream、message delta、token delta、原始 payload 或长 shell output 到 OTel logs。
- 保留当前 `feat_zff` 分支上的 workflow reasoning effort 既有改动，不在本目标里回滚或扩大处理。
- 优先服务 SigNoz 查询体验，不新增独立 UI 页面。
- 遵守本仓 `AGENTS.md`：最小影响、根因导向、先规划再执行、完成前验证。

## Stop Rule

Stop only when a final audit proves the full original outcome is complete.

Do not stop after planning, discovery, or Judge selection if a safe Worker task can be activated.

Do not stop after a single verified Worker slice when the broader owner outcome still has safe local follow-up slices. After each slice audit, advance the board to the next highest-leverage safe Worker task and continue.

Do not stop because a slice needs owner input, credentials, production access, destructive operations, or policy decisions. Mark that exact slice blocked with a receipt, create the smallest safe follow-up or workaround task, and continue all local, non-destructive work that can still move the goal toward the full outcome.

## Canonical Board

Machine truth lives at:

`docs/goals/issue-timeline-observability/state.yaml`

If this charter and `state.yaml` disagree, `state.yaml` wins for task status, active task, receipts, verification freshness, and completion truth.

## Run Command

```text
/goal Follow docs/goals/issue-timeline-observability/goal.md.
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
