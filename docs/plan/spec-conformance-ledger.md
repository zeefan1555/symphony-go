# SPEC Conformance Ledger

This ledger tracks current evidence against `SPEC.md`. Status values are `covered`, `partial`, `missing`, or `extension`.

| SPEC Range | Scenario | Status | Evidence | Next Fix |
| --- | --- | --- | --- | --- |
| 1-3 | contract_scope | covered | `docs/contract-scope.md`, `internal/app/run.go`, `internal/service/issueflow` | Keep repo-local state-write extension documented and policy-gated. |
| 4 | domain_model | covered | `internal/service/issue/types.go` | Keep new fields covered by workflow render and Linear normalization tests. |
| 5-6 | workflow_config | covered | typed workflow errors and `$VAR` tests pass in `internal/service/workflow` and `internal/runtime/config` | Keep reload smoke in real profile. |
| 7-8 | orchestrator_state | covered | normalized reconciliation and retry tests pass in `internal/service/orchestrator` | Real issue smoke remains recommended. |
| 9 | workspace_safety | covered | `internal/service/workspace` | Re-run tests after worker hook scope changes. |
| 10,12 | agent_runner | covered | one live session continuation test passes in `internal/service/issueflow` | Re-check after Codex protocol upgrades. |
| 11 | tracker_selection | covered | stable Linear error category tests pass in `internal/integration/linear` | Keep GraphQL payload category assertions in client tests. |
| 13 | observability | covered | structured dispatch log, SPEC HTTP alias, and reload-aware control tests pass | None for core. |
| 14 | failure_recovery | covered | typed workflow and Linear retry categories are covered by package tests | Keep real Linear auth failures classified in smoke runs. |
| 15 | security_ops | covered | `docs/runtime-policy.md`, `internal/service/workspace` | Keep secret and hook output tests in scope. |
| 16 | cli_lifecycle | partial | `cmd/symphony-go/main.go`, `internal/app` | Config-only startup check still needs local CLI verification. |
| 17-18 | validation_matrix | covered | targeted package tests plus `./test.sh`/`./build.sh` verification | Keep real issue smoke as an operator profile, not unit scope. |
| Appendix A | ssh_worker_optional | extension | not implemented | No action in this plan. |
