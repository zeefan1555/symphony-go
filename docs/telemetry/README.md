# Telemetry

本目录记录 `symphony-go` 接入 SigNoz + OpenTelemetry OTLP 的本地改造计划和验证口径。

## 目标

- SigNoz 成为长期观测入口。
- Traces 表达一次 issue run 的状态流转、workspace、prompt 和 Codex turn。
- Metrics 表达低基数的运行数量、阶段耗时、失败数和 token 消耗。
- Metrics 可按低基数命令类型统计 Codex turn/command 耗时，但不承载单 issue drilldown。
- Logs 保留本地 JSONL 兜底，同时补充 trace/span correlation，逐步把关键事件送入 SigNoz。

## 当前文档

- [signoz-local.md](signoz-local.md): 本地 SigNoz PoC、环境变量、分阶段 commit 计划和验收标准。
- [signoz-dashboard.md](signoz-dashboard.md): SigNoz dashboard 面板、查询字段和端到端验证步骤。
- [signoz-dashboard-panels.yaml](signoz-dashboard-panels.yaml): 固化的 dashboard 面板清单，作为 SigNoz UI 配置和查询复用的源头。

## 代码边界

- OpenTelemetry SDK 初始化只应放在 `internal/runtime/telemetry` 这类 runtime facade 内。
- 业务包不直接 import OpenTelemetry；issueflow、orchestrator、logging 通过 facade 记录语义事件。
- `internal/runtime/observability` 的 snapshot 保留为 runtime/control-plane projection，不继续承载长期历史指标语义。
- TUI 和 control `ObservabilitySnapshot` 只展示当前本地 runtime 状态；历史趋势、失败率、耗时分布和 dashboard 以 SigNoz 为准。

## 指标标签规则

Metrics label 只能使用低基数字段，例如：

- `from_state`
- `to_state`
- `phase`
- `stage`
- `step`
- `outcome`
- `error_type`
- `attempt_kind`
- `continuation`
- `command_kind`
- `command_status`

禁止把这些字段作为 metric label：

- `issue_id`
- `issue_identifier`
- `session_id`
- `thread_id`
- `turn_id`
- `turn_count`
- `duration_ms`
- `command`
- `cwd`
- `source_file`
- `source_line`
- `source_function`
- `evidence_file`
- `evidence_line`
- `evidence_locations`
- `workspace_path`

这些高基数字段只能放在 trace/log attributes 中。
