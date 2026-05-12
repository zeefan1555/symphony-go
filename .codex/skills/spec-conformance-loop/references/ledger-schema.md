# SPEC Conformance Ledger Schema

Use these schemas when creating or editing `docs/spec-conformance/*`.

## File Layout

```text
docs/spec-conformance/
  README.md
  coverage-map.md
  conformance-ledger.md
  remediation-log.md
```

Prefer Markdown tables for human review. Keep IDs stable so future agents can
resume without reinterpreting prior work.

## Stable IDs

- Source unit ID: `SPEC-<section>-<unit>`, for example `SPEC-005-003`.
- Checkpoint ID: `CHK-<section>-<unit>-<letter>`, for example
  `CHK-005-003-A`.
- Gap ID: `GAP-<area>-<number>`, for example `GAP-workflow-config-001`.
- Round ID: `ROUND-YYYYMMDD-<gap-id>`.

Use zero-padded numbers. Do not rename existing IDs after they are referenced.

## README.md Template

```md
# SPEC Conformance Loop

This directory stores the persistent state for auditing `SPEC.md` against the
current repository and remediating gaps.

## Resume Order

1. Finish `coverage-map.md` rows with `pending` or `in_progress`.
2. Finish `conformance-ledger.md` rows with `缺少证据`, `待验证`, or `未确认`.
3. Group unassigned nonconforming checkpoints into `remediation-log.md`.
4. Fix one remediation gap at a time, verify it, commit it, and log the commit.

## Current Cursor

- Phase: `<coverage|conformance|gap_grouping|remediation>`
- Next ID: `<id>`
- Last updated: `<YYYY-MM-DD>`
```

## coverage-map.md

Purpose: prove that every `SPEC.md` source unit was handled.

Columns:

| Column | Required | Meaning |
| --- | --- | --- |
| `source_id` | yes | Stable `SPEC-*` ID. |
| `spec_anchor` | yes | `SPEC.md:line` or line range. |
| `source_unit` | yes | Short summary or exact heading/list label. |
| `unit_type` | yes | `heading`, `paragraph`, `list_item`, `table`, `appendix`. |
| `disposition` | yes | How this unit is handled. |
| `checkpoint_ids` | yes | Linked `CHK-*` IDs, or `none`. |
| `status` | yes | Current processing state. |
| `notes` | yes | Rationale, especially for skipped or covered units. |

Allowed `disposition` values:

- `checkpoint`
- `background`
- `non_goal`
- `optional`
- `implementation_defined`
- `covered_by_other`

Allowed `status` values:

- `pending`
- `in_progress`
- `mapped`
- `background`
- `non_goal`
- `optional`
- `implementation_defined`
- `covered_by_other`

Template:

```md
# SPEC Coverage Map

| source_id | spec_anchor | source_unit | unit_type | disposition | checkpoint_ids | status | notes |
| --- | --- | --- | --- | --- | --- | --- | --- |
| SPEC-001-001 | `SPEC.md:16-20` | Problem statement service shape | paragraph | checkpoint | CHK-001-001-A | pending |  |
```

## conformance-ledger.md

Purpose: judge each checkpoint against repository implementation evidence.

Columns:

| Column | Required | Meaning |
| --- | --- | --- |
| `checkpoint_id` | yes | Stable `CHK-*` ID. |
| `spec_anchor` | yes | Source anchor from `SPEC.md`. |
| `规范细分点` | yes | Concrete requirement being checked. |
| `代码库实现` | yes | Current implementation summary. |
| `证据` | yes | `file:line`, test, log, or command evidence. |
| `是否符合规范` | yes | Fixed judgment value. |
| `改进建议` | yes | Next improvement if not fully conforming. |
| `gap_id` | yes | Linked `GAP-*`, or `none`. |
| `confidence` | yes | `high`, `medium`, or `low`. |

Allowed judgments:

- `符合`
- `部分符合`
- `不符合`
- `缺少证据`
- `待验证`
- `可选未实现`
- `implementation-defined 未文档化`
- `非目标`

Template:

```md
# SPEC Conformance Ledger

| checkpoint_id | spec_anchor | 规范细分点 | 代码库实现 | 证据 | 是否符合规范 | 改进建议 | gap_id | confidence |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| CHK-001-001-A | `SPEC.md:18-20` | Symphony runs issue work in isolated per-issue workspaces. | TBD | 缺少证据 | 缺少证据 | 查 workspace manager 和 orchestrator dispatch 路径。 | none | low |
```

## remediation-log.md

Purpose: track gap bundles, code rounds, commits, verification, and next steps.

Columns:

| Column | Required | Meaning |
| --- | --- | --- |
| `round_id` | yes | Stable `ROUND-*` ID. |
| `gap_id` | yes | Gap bundle ID. |
| `input_checkpoints` | yes | Comma-separated `CHK-*` IDs. |
| `status` | yes | Current gap state. |
| `root_cause` | yes | Evidence-backed root cause hypothesis. |
| `changed_files` | yes | Files touched, or `none`. |
| `commit` | yes | Local commit hash, or `none`. |
| `verification` | yes | Exact commands and results. |
| `verdict` | yes | Outcome. |
| `next_gap` | yes | Next suggested gap or `none`. |

Allowed `status` values:

- `pending`
- `in_progress`
- `verified`
- `committed`
- `blocked`
- `failed`

Allowed `verdict` values:

- `keep`
- `partial`
- `blocked`
- `revert_candidate`

Template:

```md
# SPEC Remediation Log

| round_id | gap_id | input_checkpoints | status | root_cause | changed_files | commit | verification | verdict | next_gap |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| ROUND-20260507-GAP-workflow-config-001 | GAP-workflow-config-001 | CHK-005-003-A | pending | TBD | none | none | not run | partial | none |
```

## Update Rules

- Mark a row `in_progress` before working on it.
- Never delete a completed row. Add a new row or update status/evidence.
- Do not mark a checkpoint `符合` without `file:line`, command output, test, or
  document evidence.
- Do not mark a remediation gap `committed` without a local commit hash.
- If verification fails, keep the row and set `status=failed` or `blocked` with
  the exact command and failure reason.
