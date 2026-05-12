# 本地 SigNoz + OTLP 改造计划

## 成功标准

第一轮改造完成时，需要能证明：

1. 本机 SigNoz 或等价 OTLP 测试 collector 能接收 `symphony-go` telemetry。
2. Traces 中可以按 `service.name=symphony-go` 和 `issue_identifier=ZEE-xxx` 找到一次 issue run。
3. Trace 内能看到 `Todo -> In Progress`、workspace preparation、prompt rendering、Codex turn、`AI Review -> Merging`、`Merging -> Done` 等关键节点。
4. Metrics 中能看到低基数字段聚合的 run count、transition count、phase duration、failure count、Codex turn/token 指标。
5. JSONL 日志仍可本地排障，并带有 `trace_id` / `span_id` / `phase` / `step` 关联字段。
6. Dashboard 文档能说明 active issue、retrying issue、阶段耗时、失败数、Codex turn/token 和最近失败日志的查询方式。

## 本地 SigNoz PoC

先启动 SigNoz Community Edition 自托管环境。SigNoz 自托管形态会带起 ClickHouse、OpenTelemetry Collector 等组件；本仓只对接 SigNoz 暴露的 OTLP endpoint，不直接管理底层组件。

应用侧默认使用 OTLP/gRPC：

```bash
export OTEL_SERVICE_NAME=symphony-go
export OTEL_EXPORTER_OTLP_PROTOCOL=grpc
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
export OTEL_EXPORTER_OTLP_INSECURE=true
```

如果改用 OTLP/HTTP：

```bash
export OTEL_SERVICE_NAME=symphony-go
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

自托管 SigNoz 的常见入口：

- OTLP/gRPC: `http://localhost:4317`
- OTLP/HTTP: `http://localhost:4318`

## 分 commit 实施顺序

### 1. 文档和本地 PoC

只写文档，不改 Go 行为。

预期提交：

```text
docs: document local signoz telemetry plan
```

验证：

```bash
git diff --check
```

### 2. Noop-first telemetry facade

新增 `internal/runtime/telemetry`，统一封装 provider、tracer、meter、logger、attribute 白名单和 shutdown。未配置 OTEL 时必须无副作用。

预期提交：

```text
feat(telemetry): add noop otel facade
```

验证：

```bash
./test.sh ./internal/runtime/telemetry ./internal/app
git diff --check
```

### 3. Trace issueflow trunk

先只接 issue run root span 和主干 transition：

- `Todo -> In Progress`
- `AI Review -> Merging`
- `Merging -> Done`

Trace v1 边界定义为“一次 worker dispatch / attempt 一个 trace”。跨 retry、人审等待、进程重启的 trace 串联先通过 `issue_identifier` 查询，后续再考虑持久化 traceparent。

预期提交：

```text
feat(telemetry): trace issueflow trunk
```

验证：

```bash
./test.sh ./internal/runtime/telemetry ./internal/service/issueflow ./internal/service/orchestrator
git diff --check
```

### 4. Trace workspace、prompt 和 Codex turns

在主干 trace 下补齐 workspace preparation、before/after hook、prompt rendering、Codex turn completed。Codex event 只允许白名单字段进入 telemetry，禁止导出原始 payload。

预期提交：

```text
feat(telemetry): trace workspace and codex turns
```

验证：

```bash
./test.sh ./internal/runtime/telemetry ./internal/service/issueflow ./internal/service/orchestrator
git diff --check
```

### 5. Core metrics

最小指标集：

```text
symphony_issue_run_total
symphony_issue_active
symphony_issue_retrying
symphony_issue_transition_total
symphony_issue_transition_duration_ms
symphony_issue_phase_duration_ms
symphony_issue_step_failure_total
symphony_codex_turn_total
symphony_codex_tokens_total
```

Metric label 只能使用低基数字段。`issue_id`、`issue_identifier`、`session_id`、`thread_id`、`turn_id`、`workspace_path` 只能作为 trace/log attributes。

预期提交：

```text
feat(telemetry): export core issue metrics
```

验证：

```bash
./test.sh ./internal/runtime/telemetry ./internal/service/issueflow ./internal/service/orchestrator
git diff --check
```

### 6. Logs correlation

保留 JSONL 写入路径，先补 correlation 字段：

- `trace_id`
- `span_id`
- `phase`
- `step`

关键事件可以通过 telemetry facade 发 OTel Logs，但不能削弱当前 `.symphony/logs/*.jsonl` 与 `.human.log` 的本地排障能力。

预期提交：

```text
feat(logging): correlate logs with trace context
```

验证：

```bash
./test.sh ./internal/runtime/logging ./internal/runtime/telemetry ./internal/service/orchestrator
git diff --check
```

### 7. Dashboard 和查询指南

补 dashboard 文档，覆盖：

- active issue 数
- retrying issue 数
- 各状态 issue 数
- transition 成功/失败数
- phase duration 平均值和 P95
- Codex turn/token 消耗
- 最近失败日志
- 按 `issue_identifier` drilldown 到 trace

预期提交：

```text
docs: add signoz dashboard and validation guide
```

验证：

```bash
git diff --check
```

### 8. 收窄旧观测边界

最后再把 snapshot/TUI/control observability 的语义收窄为 runtime projection，长期历史指标、耗时分布和失败率以 SigNoz 为准。不要删除本地 JSONL 兜底。

预期提交：

```text
refactor(observability): narrow snapshot to runtime projection
```

验证：

```bash
./test.sh ./internal/runtime/observability ./internal/tui ./internal/service/control ./internal/transport/hertzserver
git diff --check
```

## 代码接入锚点

- Runtime 初始化：`internal/app/run.go`
- issueflow 主干状态机：`internal/service/issueflow/flow.go`
- workspace / prompt / Codex turn：`internal/service/issueflow/agent_session.go`
- Observer 接口：`internal/service/issueflow/runtime.go`
- snapshot、token delta、日志出口：`internal/service/orchestrator/orchestrator.go`
- JSONL 事件模型：`internal/runtime/logging/jsonl.go`
- runtime snapshot：`internal/runtime/observability/snapshot.go`

## 风险控制

- 不把 OpenTelemetry SDK import 扩散到业务包。
- 不把高基数字段放进 metric labels。
- 不导出原始 Codex event payload。
- 不删除 JSONL / human log 本地兜底。
- 不绕过 `./test.sh` / `./build.sh`。
- 不在未理解当前脏区前 stage 或提交。

