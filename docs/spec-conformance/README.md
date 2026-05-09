# SPEC Conformance Loop

This directory stores the persistent state for auditing `SPEC.md` against the
current repository and remediating gaps.

## Resume Order

1. Finish `coverage-map.md` rows with `pending` or `in_progress`.
2. Finish `conformance-ledger.md` rows with `缺少证据`, `待验证`, or `未确认`.
3. Group unassigned nonconforming checkpoints into `remediation-log.md`.
4. Fix one remediation gap at a time, verify it, commit it, and log the commit.

## Current Cursor

- Phase: `remediation`
- Next ID: `GAP-http-extension-doc-001`
- Last updated: `2026-05-10`
