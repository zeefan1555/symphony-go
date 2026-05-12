# SigNoz OTLP Telemetry Retrofit

## Objective

把 `symphony-go` 的本地观测体系分阶段改造成以 SigNoz + OpenTelemetry OTLP 为长期真值的 Traces、Metrics、Logs 体系，并按可独立验证、可回滚的 commits 连续落地。

## Original Request

用户提供了 SigNoz 自托管 + `symphony-go` OTLP exporter + 本仓 telemetry facade 的改造计划，并明确希望不要按 PR 拆，而是按分 commit 提交。

## Intake Summary

- Input shape: `existing_plan`
- Audience: `symphony-go` 本地开发者和后续 `/goal` 执行 PM
- Authority: `requested`
- Proof type: `test`
- Completion proof: 本仓按分 commit 完成 telemetry facade、issueflow traces、workspace/codex traces、core metrics、log trace correlation、dashboard/validation docs、snapshot 边界收口；相关局部测试、`git diff --check`、`./build.sh` 通过，并能用本地 SigNoz 或明确的 fake/exporter 验证看到关键 trace/metric/log 证据。
- Likely misfire: 只写文档或只加零散 OpenTelemetry 调用，但没有形成可验证的 SigNoz 端到端路径；或者把 `issue_id` 放进 metric label 造成高基数风险；或者大改 snapshot/TUI 导致现有本地排障能力回退。
- Blind spots considered: 本机 SigNoz/Docker 是否可用、OTLP gRPC/HTTP 协议选择、跨 retry/human wait 的 trace 生命周期、OTel Logs Go SDK 成熟度、现有未提交改动不能覆盖、敏感 Codex payload 必须白名单。
- Existing plan facts: 保留 Phase 0 到 Phase 6 的顺序；以 `internal/runtime/telemetry` facade 隔离 OpenTelemetry；Trace 优先覆盖 `issue_run`、状态 transition、workspace/prompt/codex turn；Metrics 禁止高基数 label；Logs 先补 `trace_id/span_id` 关联再迁移；JSONL 作为本地兜底保留；最终把 snapshot/TUI 收窄为 runtime projection；提交按 commits 而非 PR 拆分。

## Goal Kind

`existing_plan`

## Current Tranche

当前 tranche 是把已有计划转成连续执行的本地仓改造：先验证计划与当前工作区，再按最小安全 commit 逐步实现。执行时不得停在“计划可行”或“第一个 slice 完成”；每个 slice 验证后要继续推进下一个安全 slice，直到最终审计证明完整改造目标达成或具体 task 被阻塞并留下可继续的替代任务。

## Non-Negotiable Constraints

- 默认使用简体中文记录面向人的说明、receipt 和必要文档。
- 不按 PR 拆；按可独立验证、可回滚的 commits 拆。
- 改动前必须检查当前脏区，不能覆盖用户已有改动。
- 不让业务代码到处直接 import OpenTelemetry；必须通过 `internal/runtime/telemetry` 或同等本仓 facade。
- Metrics label 禁止使用 `issue_id`、`issue_identifier`、`session_id`、`thread_id`、`workspace_path` 等高基数字段。
- Codex event payload 只允许白名单字段进入 Trace/Logs，避免噪音和敏感信息。
- JSONL 和本地 runtime snapshot 初期保留；最终 snapshot/TUI 只收窄语义，不做无关删除。
- 验证优先使用本仓入口：`git diff --check`、`./test.sh ...`、`./build.sh`，不要绕过仓库约定。

## Stop Rule

Stop only when a final audit proves the full original outcome is complete.

Do not stop after planning, discovery, or Judge selection if the user asked for working software or automation and a safe Worker task can be activated.

Do not stop after a single verified Worker slice when the broader owner outcome still has safe local follow-up slices. After each slice audit, advance the board to the next highest-leverage safe Worker task and continue.

Do not stop because a slice needs owner input, credentials, production access, destructive operations, or policy decisions. Mark that exact slice blocked with a receipt, create the smallest safe follow-up or workaround task, and continue all local, non-destructive work that can still move the goal toward the full outcome.

## Canonical Board

Machine truth lives at:

`docs/goals/signoz-otel-telemetry/state.yaml`

If this charter and `state.yaml` disagree, `state.yaml` wins for task status, active task, receipts, verification freshness, and completion truth.

## Run Command

```text
/goal Follow docs/goals/signoz-otel-telemetry/goal.md.
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
10. Treat each verified commit slice as a checkpoint, not completion, unless final audit explicitly proves the full original outcome is complete.
11. Finish only with a Judge/PM audit receipt that maps receipts and verification back to the original user outcome and records `full_outcome_complete: true`.
