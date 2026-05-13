# SigNoz Dashboard 和验证指南

本文档记录 `symphony-go` 接入 SigNoz 后的最小 dashboard、查询字段和端到端验收步骤。Dashboard 只使用低基数 metric label；`issue_identifier` 只用于 trace/log drilldown，不用于 metric 聚合。任务或 smoke 若要求按 `issue_identifier` 查 metrics，应记录为不适用，而不是失败。

## 前置条件

本机或测试环境已经启动 SigNoz，并按 [signoz-local.md](signoz-local.md) 配置应用环境变量：

```bash
export OTEL_SERVICE_NAME=symphony-go
export OTEL_EXPORTER_OTLP_PROTOCOL=grpc
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
export OTEL_EXPORTER_OTLP_INSECURE=true
```

运行一次真实或 fake issue 后，SigNoz 中至少应该出现：

- service: `symphony-go`
- trace attribute: `issue_identifier`
- trace span: `issue_run`
- trace span: `transition`
- trace span: `step`
- metrics: `symphony_issue_run_total`
- metrics: `symphony_issue_transition_total`
- logs fields: `trace_id`、`span_id`
- lifecycle logs: `dispatch_started`、`state_changed`、`codex_turn_completed`、`review_pass`、`push_pass`、`blocked` 或 `issue_error`

## Dashboard 面板

### 1. Active issues

数据源：Metrics

指标：

```text
symphony_issue_active
```

分组字段：

```text
state
phase
stage
```

用途：观察当前正在执行的 issue 数量。不要按 `issue_identifier` 分组；单个 issue 的细节走 Trace 查询。

### 2. Retrying issues

数据源：Metrics

指标：

```text
symphony_issue_retrying
```

分组字段：

```text
state
phase
stage
```

用途：观察当前处于 retry 队列或重试状态的 issue 数量。

### 3. Transition throughput

数据源：Metrics

指标：

```text
symphony_issue_transition_total
```

分组字段：

```text
from_state
to_state
outcome
```

用途：观察状态推进是否正常，以及失败 transition 是否集中在某条边上。

### 4. Transition duration

数据源：Metrics

指标：

```text
symphony_issue_transition_duration_ms
```

分组字段：

```text
from_state
to_state
outcome
```

建议展示：

- Average
- P95

用途：观察 tracker update、review pass、merge pass 等状态边的耗时。

### 5. Phase duration

数据源：Metrics

指标：

```text
symphony_issue_phase_duration_ms
```

分组字段：

```text
phase
step
outcome
```

建议展示：

- Average
- P95

用途：观察 workspace preparation、prompt rendering、Codex turn、hook 执行等阶段耗时。

### 6. Step failures

数据源：Metrics

指标：

```text
symphony_issue_step_failure_total
```

分组字段：

```text
phase
step
error_type
```

用途：快速定位失败主要集中在哪个阶段。`error_type` 只记录低基数类型，不放完整错误文本。

### 7. Codex turns

数据源：Metrics

指标：

```text
symphony_codex_turn_total
```

分组字段：

```text
phase
step
outcome
```

用途：观察 Codex turn 数量和成功/失败分布。

### 8. Codex tokens

数据源：Metrics

指标：

```text
symphony_codex_tokens_total
```

分组字段：

```text
token_type
```

用途：观察 input/output/total token 增量。token 口径来自 orchestrator 的 Codex event delta 逻辑。

### 9. Recent failures

数据源：Logs

过滤字段：

```text
service.name = symphony-go
outcome = error
```

可展示字段：

```text
timestamp
event
issue_identifier
phase
step
trace_id
span_id
error
```

用途：从失败日志进入对应 Trace。日志里可以包含 `issue_identifier`，因为 Logs 用于检索和 drilldown，不作为 metric label。

## Trace drilldown

### 按 issue 查一次 run

数据源：Traces

过滤字段：

```text
service.name = symphony-go
issue_identifier = ZEE-xxx
```

预期 span 结构：

```text
issue_run
├─ transition Todo -> In Progress
├─ step implementer/workspace_prepared
├─ step implementer/before_run_hook
├─ step implementer/prompt_rendered
├─ step implementer/codex_turn_completed
├─ step implementer/after_run_hook
├─ transition AI Review -> Pushing
└─ transition Pushing -> Done
```

ClickHouse 查询示例：

```sql
SELECT name, attributes_string['from_state'], attributes_string['to_state'], attributes_string['outcome']
FROM signoz_traces.signoz_index_v3
WHERE attributes_string['issue_identifier'] = 'ZEE-xxx'
ORDER BY timestamp;
```

预期至少包含：

```text
transition Todo -> In Progress
transition AI Review -> Pushing
transition Pushing -> Done
```

## Logs drilldown

SigNoz Logs 是 issue lifecycle 的统一查询入口。本地 `.human.log` / `.jsonl` 只作为本机 fallback/debug，不作为 smoke 主验收事实源。

数据源：Logs

过滤字段：

```text
service.name = symphony-go
issue_identifier = ZEE-xxx
```

ClickHouse 查询示例：

```sql
SELECT body, trace_id
FROM signoz_logs.logs_v2
WHERE attributes_string['issue_identifier'] = 'ZEE-xxx'
ORDER BY timestamp;
```

预期包含当前 issue 的 lifecycle logs：`dispatch_started`、`state_changed`、`codex_turn_completed`，以及对应收口阶段的 `review_pass` / `push_pass`。若出现 blocker 或错误，应能查到 `blocked` 或 `issue_error` 等事件。

### 从日志跳回 Trace

数据源：Logs

过滤字段：

```text
trace_id = <jsonl trace_id>
```

预期结果：

- 日志事件带 `trace_id` 和 `span_id`。
- 同一 `trace_id` 能在 Traces 里找到 `issue_run`。
- 失败日志可通过 `phase` / `step` 对应到具体 span。

## 验收步骤

1. 启动 SigNoz。
2. 导出 OTLP 环境变量。
3. 启动 `symphony-go`。
4. 运行一个 fake 或真实 issue，让它至少经过 implementation 阶段。
5. 在 Traces 中按 `service.name=symphony-go` 查询。
6. 在 Traces 中按 `issue_identifier=ZEE-xxx` drilldown。
7. 在 Metrics 中确认核心指标出现。
8. 在 Logs 中按 `issue_identifier=ZEE-xxx` 确认 lifecycle logs 存在，并带 `trace_id` / `span_id`。
9. 用任意一条失败或关键日志的 `trace_id` 回查 Trace。
10. 在 Metrics 中不带 `issue_identifier` 查询低基数字段：

```sql
SELECT metric_name, attrs
FROM signoz_metrics.time_series_v4
WHERE metric_name LIKE 'symphony_%'
LIMIT 20;
```

验收点是存在 `symphony_issue_transition_total`，并能看到 `from_state` / `to_state` / `outcome` 等低基数字段；不要求、也不允许要求 `issue_identifier`。

## 禁止项

- Metric label 不允许使用 `issue_id`、`issue_identifier`、`session_id`、`thread_id`、`turn_id`、`workspace_path`。
- 不把原始 Codex event payload 直接写入 Trace、Metric 或 Log。
- 不删除 JSONL 本地兜底；SigNoz 是长期查询入口，JSONL 是离线排障和本地兜底。
