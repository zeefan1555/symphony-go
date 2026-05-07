# SPEC Coverage Map

| source_id | spec_anchor | source_unit | unit_type | disposition | checkpoint_ids | status | notes |
| --- | --- | --- | --- | --- | --- | --- | --- |
| SPEC-000-001 | `SPEC.md:1` | Symphony Service Specification title | heading | background | none | background | Top-level document title; no direct implementation judgment. |
| SPEC-000-002 | `SPEC.md:3` | Status: Draft v1 (language-agnostic) | paragraph | background | none | background | Document maturity marker; no implementation judgment. |
| SPEC-000-003 | `SPEC.md:5` | Purpose: define an agent orchestration service | paragraph | background | none | background | Scope statement for the specification; concrete requirements are split in later sections. |
| SPEC-000-004 | `SPEC.md:7` | Normative Language heading | heading | background | none | background | Section heading only. |
| SPEC-000-005 | `SPEC.md:9-10` | RFC 2119 keyword interpretation | paragraph | background | none | background | Interpretation rule for requirement strength; no repository implementation checkpoint. |
| SPEC-000-006 | `SPEC.md:12-14` | Implementation-defined behavior must document selected policy | paragraph | implementation_defined | CHK-000-006-A | implementation_defined | Check that implementation-defined runtime choices are explicitly documented. |
| SPEC-001-001 | `SPEC.md:16` | 1. Problem Statement heading | heading | background | none | background | Section heading only. |
| SPEC-001-002 | `SPEC.md:18-20` | Long-running Linear reader creates isolated workspaces and runs coding agents inside them | paragraph | checkpoint | CHK-001-002-A, CHK-001-002-B, CHK-001-002-C | mapped | Split into service loop, workspace, and agent cwd checkpoints. |
| SPEC-001-003 | `SPEC.md:22` | Service solves four operational problems | paragraph | background | none | background | Lead-in sentence; concrete bullets are mapped separately. |
| SPEC-001-004 | `SPEC.md:24` | Repeatable daemon workflow instead of manual scripts | list_item | checkpoint | CHK-001-004-A | mapped | Check listener loop and repo runtime assembly. |
| SPEC-001-005 | `SPEC.md:25-26` | Agent execution is isolated in per-issue workspace directories | list_item | checkpoint | CHK-001-005-A | mapped | Check workspace path containment and Codex cwd. |
| SPEC-001-006 | `SPEC.md:27-28` | Workflow policy is in-repo `WORKFLOW.md` | list_item | checkpoint | CHK-001-006-A | mapped | Check workflow loader and repo workflow. |
| SPEC-001-007 | `SPEC.md:29` | Observability supports debugging concurrent agent runs | list_item | checkpoint | CHK-001-007-A | mapped | Check structured logs and snapshot state. |
| SPEC-001-008 | `SPEC.md:31-34` | Trust and safety posture must be documented explicitly | paragraph | covered_by_other | CHK-000-006-A | covered_by_other | Covered by implementation-defined runtime policy checkpoint. |
| SPEC-001-009 | `SPEC.md:36` | Important boundary heading | heading | background | none | background | Section heading only. |
| SPEC-001-010 | `SPEC.md:38` | Symphony is scheduler/runner and tracker reader | list_item | checkpoint | CHK-001-010-A | mapped | Check contract scope and runtime assembly boundaries. |
| SPEC-001-011 | `SPEC.md:39-40` | Ticket writes are typically performed by coding agent tooling | list_item | checkpoint | CHK-001-011-A | mapped | Check policy boundary and `linear_graphql` workflow ownership. |
| SPEC-001-012 | `SPEC.md:41-42` | Successful run can end at workflow-defined handoff state, not necessarily `Done` | list_item | checkpoint | CHK-001-012-A | mapped | Check workflow-defined handoff and contract-scope documentation. |
| SPEC-002-001 | `SPEC.md:44` | 2. Goals and Non-Goals heading | heading | background | none | background | Section heading only. |
| SPEC-002-002 | `SPEC.md:46` | 2.1 Goals heading | heading | background | none | background | Section heading only. |
| SPEC-002-003 | `SPEC.md:48` | Poll issue tracker on fixed cadence and dispatch with bounded concurrency | list_item | checkpoint | CHK-002-003-A, CHK-002-003-B | mapped | Split into cadence and concurrency checkpoints. |
| SPEC-002-004 | `SPEC.md:49` | Single authoritative orchestrator state for dispatch, retries, and reconciliation | list_item | checkpoint | CHK-002-004-A | mapped | Check orchestrator-owned maps and snapshot state. |
| SPEC-002-005 | `SPEC.md:50` | Deterministic per-issue workspaces are preserved across runs | list_item | checkpoint | CHK-002-005-A | mapped | Check workspace path derivation and no cleanup on normal active run. |
| SPEC-002-006 | `SPEC.md:51` | Stop active runs when issue state changes make them ineligible | list_item | checkpoint | CHK-002-006-A | mapped | Check reconciliation cancellation. |
| SPEC-002-007 | `SPEC.md:52` | Recover transient failures with exponential backoff | list_item | checkpoint | CHK-002-007-A | mapped | Check retry delay and scheduleRetry. |
| SPEC-002-008 | `SPEC.md:53` | Load runtime behavior from repository-owned `WORKFLOW.md` contract | list_item | covered_by_other | CHK-001-006-A | covered_by_other | Same requirement covered in Section 1 workflow policy checkpoint. |
| SPEC-002-009 | `SPEC.md:54` | Expose operator-visible observability, at minimum structured logs | list_item | covered_by_other | CHK-001-007-A | covered_by_other | Same requirement covered in Section 1 observability checkpoint. |
| SPEC-002-010 | `SPEC.md:55-56` | Restart recovery uses tracker/filesystem without persistent database; exact in-memory scheduler state is not restored | list_item | checkpoint | CHK-002-010-A | mapped | Check startup terminal cleanup and in-memory runtime state. |
| SPEC-002-011 | `SPEC.md:58` | 2.2 Non-Goals heading | heading | background | none | background | Section heading only. |
| SPEC-002-012 | `SPEC.md:60` | Rich web UI or multi-tenant control plane is non-goal | list_item | non_goal | CHK-002-012-A | non_goal | Check contract-scope optional surface boundary. |
| SPEC-002-013 | `SPEC.md:61` | Specific dashboard or terminal UI implementation is not prescribed | list_item | non_goal | CHK-002-013-A | non_goal | Check TUI/control plane are optional surfaces. |
| SPEC-002-014 | `SPEC.md:62` | General-purpose workflow engine or distributed job scheduler is non-goal | list_item | non_goal | CHK-002-014-A | non_goal | Check contract-scope boundary. |
| SPEC-002-015 | `SPEC.md:63-64` | Built-in business logic for editing tickets, PRs, or comments is non-goal | list_item | covered_by_other | CHK-001-011-A | covered_by_other | Same boundary covered by ticket-write checkpoint. |
| SPEC-002-016 | `SPEC.md:65` | Strong sandbox controls beyond agent and host OS are non-goal | list_item | covered_by_other | CHK-000-006-A | covered_by_other | Covered by runtime policy/trust boundary checkpoint. |
| SPEC-002-017 | `SPEC.md:66-67` | Single default approval, sandbox, or operator-confirmation posture is non-goal | list_item | covered_by_other | CHK-000-006-A | covered_by_other | Covered by implementation-defined runtime policy checkpoint. |
