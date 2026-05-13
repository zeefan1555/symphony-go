# ZEE-178 Issue Timeline Observability Smoke

## Scope

- Issue: `ZEE-178`
- Target branch: `feat_zff`
- Baseline commit: `b01dbb1`
- Smoke start: `2026-05-13T12:20:43Z`
- Repo root: `/Users/bytedance/symphony-go`

This receipt records the local evidence for the live issueflow smoke after
`b01dbb1`. The durable execution log for the run is the single Linear
`## Codex Workpad` comment on `ZEE-178`.

## Expected Timeline Evidence

SigNoz traces and logs should be queried with `issue_identifier=ZEE-178`.
The run is expected to expose the normal issueflow lifecycle:

```text
Todo -> In Progress -> AI Review -> Pushing -> Done
```

For issue timeline drilldown, use traces and logs. Metrics remain global and
low-cardinality; do not require `issue_identifier` as a metric label.

## SigNoz Query Anchors

Trace timeline:

```sql
SELECT
  timestamp,
  name,
  duration_nano / 1000000 AS duration_ms,
  attributes_string['phase'] AS phase,
  attributes_string['stage'] AS stage,
  attributes_string['from_state'] AS from_state,
  attributes_string['to_state'] AS to_state,
  attributes_string['outcome'] AS outcome
FROM signoz_traces.signoz_index_v3
WHERE attributes_string['issue_identifier'] = 'ZEE-178'
ORDER BY timestamp;
```

Logs timeline:

```sql
SELECT
  timestamp,
  attributes_string['event'] AS event,
  body,
  attributes_number['duration_ms'] AS duration_ms,
  trace_id,
  span_id
FROM signoz_logs.logs_v2
WHERE attributes_string['issue_identifier'] = 'ZEE-178'
ORDER BY timestamp;
```

## Local Validation

Minimum local validation for this smoke:

```bash
git diff --check
./test.sh ./internal/runtime/telemetry ./internal/service/issueflow
```

