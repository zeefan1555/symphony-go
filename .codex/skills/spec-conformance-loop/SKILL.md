---
name: spec-conformance-loop
description: Drive a persistent SPEC.md conformance audit and remediation loop for this repo. Use when the user asks to split SPEC.md, compare it against the codebase, maintain coverage/conformance ledgers, or iteratively fix SPEC gaps.
---

# SPEC Conformance Loop

This skill is scoped to `/Users/bytedance/symphony-go`.

Use it to turn `SPEC.md` into a complete, persistent audit and remediation loop.
The goal is not a one-off gap summary. The goal is to ensure every design point
in `SPEC.md` is covered, checked against the current repository, and converted
into small verified improvements.

## Required Inputs

Read these first:

- `AGENTS.md`
- `CONTEXT.md`
- `SPEC.md`
- `WORKFLOW.md`
- `docs/agents/domain.md`
- Existing ledgers under `docs/spec-conformance/`, if present

Do not use `docs/architecture/symphony-go-architecture.md` as a source of
truth unless the user explicitly asks for historical comparison.

## Persistent Files

Create these lazily when the loop first needs them:

```text
docs/spec-conformance/
  README.md
  coverage-map.md
  conformance-ledger.md
  remediation-log.md
```

- `README.md`: the repo-local SOP and resume protocol.
- `coverage-map.md`: complete top-to-bottom handling of `SPEC.md` source units.
- `conformance-ledger.md`: checkpoint-by-checkpoint implementation evidence.
- `remediation-log.md`: gap bundles, code rounds, commits, and verification.

See `references/ledger-schema.md` for required columns, status values, and
templates. Load it before creating or editing any ledger.

## Resume Protocol

Every run starts by restoring state:

1. Run `git status --short --branch`.
2. Read the four files under `docs/spec-conformance/` if they exist.
3. Identify the next unfinished row using this priority:
   - Coverage rows with `pending` or `in_progress`.
   - Conformance rows with `缺少证据`, `待验证`, or `未确认`.
   - Nonconforming rows not yet assigned to a `gap_id`.
   - Remediation gaps with `pending`, `in_progress`, or `failed`.
4. Continue from that row. Do not restart from scratch unless the user asks.

If unrelated local changes exist, leave them untouched. If they block the next
row, stop and report the conflict.

## Phase Order

The phases are mandatory and ordered:

1. `Coverage Loop`
2. `Conformance Loop`
3. `Gap Grouping`
4. `Remediation Loop`

Do not compare code or propose fixes until the relevant `SPEC.md` source unit is
recorded in `coverage-map.md`. Do not modify code until the corresponding gap is
recorded in `remediation-log.md`.

## Phase 1: Coverage Loop

Process `SPEC.md` from top to bottom by source unit: headings, paragraphs, list
items, tables, and appendix sections.

Each source unit must get one disposition:

- `checkpoint`: split into one or more verifiable checkpoints.
- `background`: explanatory context, no direct implementation judgment.
- `non_goal`: explicit non-goal.
- `optional`: optional capability.
- `implementation_defined`: implementation must document its selected policy.
- `covered_by_other`: covered by another source unit; link the checkpoint ID.

Mark the current row `in_progress` before working. Mark it complete only after
the row records the source anchor, disposition, and checkpoint IDs or rationale.

## Phase 2: Conformance Loop

For each checkpoint, inspect repository evidence before judging:

- Code anchors such as `internal/service/...`, `internal/runtime/...`,
  `internal/integration/...`, `internal/transport/...`, `internal/app/...`, and
  `cmd/symphony-go/main.go`.
- Documentation anchors such as `CONTEXT.md`, `WORKFLOW.md`, `docs/adr/`,
  `docs/runtime-policy.md`, and `docs/contract-scope.md`.
- Tests and command output when behavior must be proven.

A row cannot be `符合` without evidence. If evidence is absent or ambiguous,
write `缺少证据` or `未确认` rather than guessing.

## Phase 3: Gap Grouping

Do not turn every failed checkpoint into a separate task. Group related
nonconforming checkpoints into one small `gap_id` when they share the same root
cause or implementation area.

Each gap must record:

- affected checkpoint IDs
- root cause hypothesis
- target files or package boundary
- smallest safe fix
- required verification commands

## Phase 4: Remediation Loop

Work one `gap_id` at a time.

Before editing code:

1. Re-read the relevant spec anchors and code anchors.
2. Add or update failing tests when the gap is behavioral.
3. State the minimal change and verification commands.

After editing:

1. Run the smallest relevant verification first.
2. Run `git diff --check`.
3. Run broader `./test.sh` or `./build.sh` only when impact justifies it.
4. Update `conformance-ledger.md` and `remediation-log.md`.
5. Create one local commit for the completed gap bundle.
6. Record the commit hash in `remediation-log.md`.

Use repo scripts for Go verification. Do not replace them with bare `go test` or
`go build` unless diagnosing the scripts themselves.

## Completion Criteria

A run is complete only when one minimal loop is closed and persisted:

- Coverage work: updated `coverage-map.md`.
- Conformance work: updated `conformance-ledger.md` with evidence.
- Gap grouping: updated `remediation-log.md` with a `gap_id`.
- Remediation work: code changes verified, committed, and logged.

Final replies must include changed files, verification commands and results,
and the next unfinished row or state that no unfinished rows remain.
