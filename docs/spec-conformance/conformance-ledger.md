# SPEC Conformance Ledger

| checkpoint_id | spec_anchor | 规范细分点 | 代码库实现 | 证据 | 是否符合规范 | 改进建议 | gap_id | confidence |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| CHK-000-006-A | `SPEC.md:12-14` | Implementation-defined 行为必须记录本实现选择的策略。 | `docs/runtime-policy.md` 记录 high-trust 本地运行边界、approval/sandbox 转发、当前 `turn_sandbox_policy`、无人值守 operator-confirmation fail-fast、secret、hook、dynamic tool 与 issueflow 写入策略。 | `docs/runtime-policy.md:3`, `docs/runtime-policy.md:7-23`, `docs/runtime-policy.md:39-58`, `internal/app/contract_scope_test.go:61-84` | 符合 | 已通过 `GAP-runtime-policy-001` 补齐当前 turn sandbox 与 operator-confirmation 策略描述。 | GAP-runtime-policy-001 | high |
