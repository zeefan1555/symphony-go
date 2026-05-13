# T001 Repo Map

## Findings

- `issue_run` parent span root cause is in `internal/runtime/telemetry/issue_run.go:11`: `StartIssueRun` starts `issue_run` and immediately ends it at line 14. `RunIssueTrunk` already opens it at `internal/service/issueflow/flow.go:12` and defers `endIssueRun` at line 14, so the caller shape is already correct but the facade closes too early.
- Duration-bearing step support already exists in `StartStep` at `internal/runtime/telemetry/issue_run.go:43`, but `RecordStep` at line 59 is instant-only and records metrics with `0` duration at line 65. `codex_turn_completed` currently uses this instant path in `internal/service/issueflow/agent_session.go:77`.
- `codex.Result` currently has only IDs and process metadata at `internal/service/codex/runner.go:53`. `RunSession` emits `turn_started` and `turn_completed` payloads at `internal/service/codex/runner.go:168` and `internal/service/codex/runner.go:185`, but they do not include `started_at`, `completed_at`, or `duration_ms`.
- `AfterTurn` records `codex_turn_completed` with the outer closure `phase` at `internal/service/issueflow/agent_session.go:77` and logs with the same phase at line 84. Continuation phase/stage should instead be derived from `turnStartIssue.State` and `nextWorkerStage(turnStartIssue, turnCount)`.
- OTel logs are curated by `internal/runtime/telemetry/logs.go:14`. `codex_turn_completed` is exported, but `codex_turn_started`, `codex_final`, `codex_command`, and `codex_file_change` are not. The field allowlist at line 37 also lacks `command`, `cwd`, `exit_code`, `duration_ms`, `turn_count`, `continuation`, and `turn_id`.
- Metric cardinality guard already exists in `internal/runtime/telemetry/attrs.go:10` and blocks `issue_id`, `issue_identifier`, `session_id`, `thread_id`, `turn_id`, and `workspace_path` from metric labels.
- Existing docs already state that `issue_identifier` belongs to trace/log drilldown, not metrics, in `docs/telemetry/signoz-dashboard.md:3`; the timeline examples need to be extended after implementation.

## Recommended First Worker Slice

Implement T003 first:

- Objective: keep `issue_run` open until `endIssueRun` is called, set final `outcome`, record span status/error, then end the span and emit the run metric.
- Allowed files:
  - `internal/runtime/telemetry/issue_run.go`
  - `internal/runtime/telemetry/provider_test.go`
  - `internal/service/issueflow/flow_test.go`
- Verify:
  - `./test.sh ./internal/runtime/telemetry -run 'TestStartIssueRunAndRecordTransitionCreateSpans|TestIssueRunSpanEndsAfterChildTransition'`
  - `./test.sh ./internal/service/issueflow -run 'TestRunIssueTrunkRecordsIssueRunAndTodoTransitionSpans|TestRunIssueTrunkRecordsWorkspacePromptAndTurnSpans'`
  - `git diff --check`
- Stop if the change requires rewriting issueflow orchestration or adding high-cardinality metric labels.
