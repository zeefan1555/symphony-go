# Symphony Go SPEC Gap Remediation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring current `/Users/bytedance/symphony-go` `main` into clearer conformance with `SPEC.md` core requirements, while keeping Symphony Go's workflow-specific issueflow automation isolated as an explicit extension.

**Architecture:** Preserve the current long-running local daemon shape and existing package boundaries: `internal/runtime/config`, `internal/service/workflow`, `internal/integration/linear`, `internal/service/orchestrator`, `internal/service/workspace`, `internal/service/codex`, and `internal/service/issueflow`. The main correction is not a broad rewrite; it is to make worker sessions, state normalization, error surfaces, tracker-write boundaries, and HTTP extension compatibility match the spec contracts already described in `SPEC.md`.

**Tech Stack:** Go, `gopkg.in/yaml.v3`, `github.com/osteele/liquid`, Codex app-server over stdio JSON lines, Linear GraphQL, existing Hertz-generated control plane, existing repo scripts `./test.sh`, `./build.sh`, and `git diff --check`.

---

## Current Baseline

This plan supersedes the stale assumptions in `docs/plan/go-symphony-v3-core.md`, which still describes old `go/internal/...` paths and a pre-refactor v1 shape. Current `main` already has the core package split, runtime config, workflow reload, retry/reconciliation scaffolding, workspace hooks, optional `linear_graphql`, and a Hertz control plane.

Evidence:

| Area | SPEC Anchor | Current Code Anchor | Verdict |
| --- | --- | --- | --- |
| Core components exist | `SPEC.md:71-113` | `docs/contract-scope.md:9-16`, `internal/app/run.go:17-27` | Mostly covered |
| Typed config exists | `SPEC.md:507-607` | `internal/runtime/config/config.go:52-70`, `internal/runtime/config/types.go:10-20` | Partial |
| Dynamic reload exists | `SPEC.md:532-550` | `internal/service/workflow/reloader.go:73-94`, `internal/service/orchestrator/orchestrator.go:540-568` | Partial |
| Workspace safety exists | `SPEC.md:820-915` | `internal/service/workspace/workspace.go:81-143`, `internal/service/workspace/workspace.go:191-241` | Mostly covered |
| Orchestrator state exists | `SPEC.md:608-705` | `internal/service/orchestrator/orchestrator.go:58-70`, `internal/service/orchestrator/orchestrator.go:646-745` | Partial |
| App-server client exists | `SPEC.md:916-1142` | `internal/service/codex/runner.go:124-197`, `internal/service/codex/runner.go:199-241` | Partial |
| Linear reader exists | `SPEC.md:1143-1209` | `internal/integration/linear/client.go:135-200`, `internal/integration/linear/client.go:426-457` | Partial |
| Observability exists | `SPEC.md:1255-1344` | `internal/runtime/observability/snapshot.go:5-14`, `internal/service/orchestrator/orchestrator.go:1025-1058` | Mostly covered |
| HTTP extension exists | `SPEC.md:1354-1525` | `idl/main.proto:17-69`, `internal/transport/hertzserver/server.go:31-42` | Extension shape diverges |

## Main Gaps

### Gap 1: Worker Continuation Does Not Reuse One Live App-Server Thread

`SPEC.md` requires continuation turns inside one worker to reuse the same live `thread_id` and avoid resending the original task prompt (`SPEC.md:635-647`, `SPEC.md:951-978`, `SPEC.md:992-997`). The runner supports multi-turn sessions through `codex.SessionRequest.Prompts` and `AfterTurn` (`internal/service/codex/runner.go:58-63`, `internal/service/codex/runner.go:124-197`), but `issueflow.runAgentTurn` starts a new `RunSession` for each phase/turn (`internal/service/issueflow/agent_session.go:14-85`), and `flow.go` loops by calling `runAgentTurn` again (`internal/service/issueflow/flow.go:84-104`, `internal/service/issueflow/flow.go:106-178`). This means continuation wording says "same issue session", but the implementation starts a new app-server process/thread per turn.

Impact: session metrics, stall detection, context carry-over, and continuation prompt semantics are not spec-faithful.

### Gap 2: Hooks Are Scoped Per Turn Instead Of Per Worker Attempt

`SPEC.md` says `before_run` runs before each agent attempt and `after_run` runs after each attempt (`SPEC.md:871-895`, `SPEC.md:1815-1872`). Current `runAgentTurn` defers `AfterRun` and calls `BeforeRun` inside each turn helper (`internal/service/issueflow/agent_session.go:20-36`). Once worker continuation is fixed to use one live session, hooks should wrap the whole worker attempt, not each turn.

Impact: a multi-turn worker can repeatedly run setup/teardown scripts, which is observable and can be destructive if hooks sync worktrees or mutate files.

### Gap 3: Tracker Writes Are Mixed Into Core Issueflow

`SPEC.md` defines Symphony as a scheduler/runner and tracker reader, and says ticket writes are typically performed by the coding agent and workflow tooling (`SPEC.md:36-42`, `SPEC.md:63-64`, `SPEC.md:1210-1220`). Current code performs workflow-specific state writes inside issueflow: `Todo -> In Progress` (`internal/service/issueflow/flow.go:64-73`), `AI Review -> Merging` (`internal/service/issueflow/flow.go:122-128`), and `Merging -> Done` (`internal/service/issueflow/flow.go:159-165`). `docs/contract-scope.md` currently documents this as a narrow runtime control exception (`docs/contract-scope.md:18-23`).

Impact: core conformance and workflow extension behavior are coupled. This is acceptable only if the extension boundary is explicit, testable, and not required for the scheduler/runner contract.

### Gap 4: State Normalization Is Inconsistent

`SPEC.md` requires normalized issue-state comparison after lowercase (`SPEC.md:275-287`). Candidate eligibility uses `EqualFold` (`internal/service/orchestrator/eligibility.go:118-125`), but reconciliation helpers compare exact strings (`internal/service/orchestrator/orchestrator.go:908-920`) and issueflow helpers also compare exact strings (`internal/service/issueflow/flow.go:234-253`).

Impact: state casing drift from Linear or workflow config can stop dispatch/reconciliation decisions from matching the spec.

### Gap 5: Workflow/Config Error Surface Is Not Typed Enough

`SPEC.md` names error classes such as `missing_workflow_file`, `workflow_parse_error`, `workflow_front_matter_not_a_map`, `template_parse_error`, and `template_render_error` (`SPEC.md:492-506`, `SPEC.md:1941-1961`). Current workflow loader returns generic wrapped errors for read and YAML failures (`internal/service/workflow/workflow.go:28-44`), and config has typed codes for validation errors (`internal/runtime/config/config.go:13-24`) but not for workflow loader classes.

Impact: startup and reload errors are operator-visible, but downstream tests/control surfaces cannot reliably classify all spec-defined workflow failures.

### Gap 6: `$VAR` Resolution Does More Than The Spec Allows

`SPEC.md` says environment variables do not globally override YAML and are only used when config explicitly references `$VAR_NAME` (`SPEC.md:511-520`). Current config resolution additionally falls back to `LINEAR_API_KEY` when `tracker.api_key` is empty (`internal/runtime/config/config.go:169-173`), while `docs/runtime-policy.md` describes explicit `$VAR` indirection as the intended policy (`docs/runtime-policy.md:23-27`).

Impact: behavior is convenient but not spec-faithful. The repo workflow already uses `api_key: $LINEAR_API_KEY` (`WORKFLOW.md:2-5`), so tightening this should not break the checked-in default workflow.

### Gap 7: Linear Error Mapping Is Mostly String-Based

`SPEC.md` recommends stable Linear error categories such as `linear_api_request`, `linear_api_status`, `linear_graphql_errors`, `linear_unknown_payload`, and `linear_missing_end_cursor` (`SPEC.md:1191-1209`). Current Linear client returns plain `error` strings for request/status/GraphQL cases (`internal/integration/linear/client.go:288-352`) and only has a string literal for missing pagination cursor (`internal/integration/linear/client.go:163-165`).

Impact: retry logic can continue working, but observability, control API error envelopes, and tests cannot make stable category assertions.

### Gap 8: HTTP Extension Does Not Expose The Baseline SPEC Routes

The optional HTTP extension, if implemented, should provide `GET /api/v1/state`, `GET /api/v1/<issue_identifier>`, and `POST /api/v1/refresh` baseline ergonomics (`SPEC.md:1391-1525`). Current Hertz IDL exposes POST action-style routes such as `/api/v1/control/get-state` and `/api/v1/control/refresh` (`idl/main.proto:17-25`), which is intentionally documented as the repo's stable generated API style (`docs/control-plane-hertz-idl.md:59-63`).

Impact: this is an extension-conformance gap, not a core scheduler gap. The least disruptive fix is to add compatibility aliases around the existing service without breaking generated routes.

### Gap 9: Runtime Control Service Captures Some Initial Dependencies

`NewRuntime` builds a control service with the initial workspace manager, runner, tracker, and config (`internal/app/run.go:161-171`). Orchestrator reload swaps these dependencies for scheduling (`internal/service/orchestrator/orchestrator.go:551-564`), but the already-created control service can still project initial config and helper dependencies.

Impact: after workflow reload, the scheduler can use new config while control-plane diagnostics may show or use stale settings.

### Gap 10: Console Output Bypasses Structured Logging

`SPEC.md` requires operator-visible observability and structured logs for runtime events (`SPEC.md:1255-1285`, `SPEC.md:2037-2047`). Current dispatch prints issue lines directly with `fmt.Printf` (`internal/service/orchestrator/orchestrator.go:207`) in addition to the structured logger.

Impact: small but noisy. It can make tests and unattended output less deterministic and bypasses `issue_id` / `issue_identifier` structured context.

## Non-Goals For This Remediation

- Do not implement Appendix A SSH workers.
- Do not add durable retry/session persistence; `SPEC.md` lists it as an extension item (`SPEC.md:2105`).
- Do not replace the Hertz-generated control plane or move handwritten business logic into `gen/hertz/...`.
- Do not redesign `WORKFLOW.md` semantics beyond the tracker-write boundary needed for conformance.
- Do not remove the existing `linear_graphql` extension; it already matches most of `SPEC.md:1066-1097` via `internal/service/codex/dynamic_tool.go:56-86`.

## Task 1: Add A Current Conformance Ledger

**Files:**
- Create: `docs/plan/spec-conformance-ledger.md`
- Modify: none outside docs
- Validate: `git diff --check`

- [ ] **Step 1: Create the ledger skeleton**

Write `docs/plan/spec-conformance-ledger.md` with these rows:

```markdown
# SPEC Conformance Ledger

This ledger tracks current evidence against `SPEC.md`. Status values are `covered`, `partial`, `missing`, or `extension`.

| SPEC Range | Scenario | Status | Evidence | Next Fix |
| --- | --- | --- | --- | --- |
| 1-3 | contract_scope | partial | `docs/contract-scope.md`, `internal/app/run.go` | Separate core reader/scheduler behavior from issueflow tracker-write extension. |
| 4 | domain_model | covered | `internal/service/issue/types.go` | Keep new fields covered by workflow render and Linear normalization tests. |
| 5-6 | workflow_config | partial | `internal/service/workflow`, `internal/runtime/config` | Add typed workflow errors and strict `$VAR` semantics. |
| 7-8 | orchestrator_state | partial | `internal/service/orchestrator` | Fix normalized state comparisons and same-worker continuation. |
| 9 | workspace_safety | covered | `internal/service/workspace` | Re-run tests after worker hook scope changes. |
| 10,12 | agent_runner | partial | `internal/service/codex`, `internal/service/issueflow` | Reuse one live app-server thread for worker continuation. |
| 11 | tracker_selection | partial | `internal/integration/linear` | Add stable Linear error categories. |
| 13 | observability | partial | `internal/runtime/observability`, `internal/runtime/logging` | Remove raw `fmt.Printf`; make control-plane state reload-aware. |
| 14 | failure_recovery | partial | `internal/service/orchestrator` | Confirm typed errors and retry categories in tests. |
| 15 | security_ops | covered | `docs/runtime-policy.md`, `internal/service/workspace` | Keep secret and hook output tests in scope. |
| 16 | cli_lifecycle | partial | `cmd/symphony-go/main.go`, `internal/app` | Add missing workflow startup error classification tests. |
| 17-18 | validation_matrix | partial | package tests plus smoke docs | Add targeted tests for each remediation task. |
| Appendix A | ssh_worker_optional | extension | not implemented | No action in this plan. |
```

- [ ] **Step 2: Validate documentation formatting**

Run:

```bash
git diff --check
```

Expected: no whitespace errors.

## Task 2: Make Worker Continuation Use One Live App-Server Session

**Files:**
- Modify: `internal/service/issueflow/agent_session.go`
- Modify: `internal/service/issueflow/flow.go`
- Modify: `internal/service/issueflow/runtime.go`
- Modify: `internal/service/issueflow/flow_test.go`
- Modify if needed: `internal/service/codex/runner.go`
- Test: `./test.sh ./internal/service/issueflow ./internal/service/orchestrator ./internal/service/codex`

- [ ] **Step 1: Add a failing issueflow test for thread reuse**

Add a test in `internal/service/issueflow/flow_test.go` that uses a fake runner recording `RunSession` calls. Configure the fake so `AfterTurn` returns a second continuation prompt while the issue remains active. Assert:

```go
if runner.sessionCalls != 1 {
    t.Fatalf("RunSession calls = %d, want one live session for worker continuation", runner.sessionCalls)
}
if got := runner.prompts; !reflect.DeepEqual(got, []string{initialPrompt, ContinuationPromptText}) {
    t.Fatalf("prompts = %#v, want initial prompt then continuation prompt", got)
}
```

Run:

```bash
./test.sh ./internal/service/issueflow -run TestRunIssueTrunkReusesOneSessionForContinuation
```

Expected before implementation: fail because current `runAgentTurn` calls `RunSession` per turn.

- [ ] **Step 2: Move worker-attempt orchestration into one helper**

Replace per-turn `runAgentTurn` orchestration with one worker helper that:

1. Calls `Workspace.Ensure` once for the worker attempt.
2. Calls `Workspace.BeforeRun` once before launching Codex.
3. Builds the first prompt from `workflow.Render`.
4. Calls `Runner.RunSession` once with `AfterTurn`.
5. In `AfterTurn`, refreshes the issue, decides whether to stop, or returns `ContinuationPromptText`, `AIReviewContinuationPromptText`, or `MergingContinuationPromptText`.
6. Calls `Workspace.AfterRun` once in a deferred best-effort path after the session attempt ends.

The helper should preserve the existing state-specific final-message checks:

```go
reviewFinalPasses(lastAgentMessage) // triggers AI Review -> Merging extension behavior
mergeFinalPasses(lastAgentMessage)  // triggers Merging -> Done extension behavior
```

- [ ] **Step 3: Keep runner multi-turn API unchanged**

Do not broaden `codex.Runner` unless the failing test proves a runner bug. The existing `SessionRequest.AfterTurn` path already appends prompts inside one subprocess/thread (`internal/service/codex/runner.go:184-195`).

- [ ] **Step 4: Run targeted tests**

Run:

```bash
./test.sh ./internal/service/issueflow ./internal/service/orchestrator ./internal/service/codex
```

Expected: pass. Evidence must include a test that proves exactly one `RunSession` call for a multi-turn worker.

## Task 3: Correct Hook Scope Around Worker Attempts

**Files:**
- Modify: `internal/service/issueflow/agent_session.go`
- Modify: `internal/service/workspace/workspace_test.go`
- Modify: `internal/service/orchestrator/orchestrator_test.go`
- Test: `./test.sh ./internal/service/issueflow ./internal/service/workspace ./internal/service/orchestrator`

- [ ] **Step 1: Add a failing hook-count test**

Create a fake workspace manager or hook observer test that runs a two-turn worker and asserts:

```go
want := []string{"before_run", "after_run"}
if got := hookNames; !reflect.DeepEqual(got, want) {
    t.Fatalf("hooks = %#v, want %#v for one worker attempt", got, want)
}
```

Run:

```bash
./test.sh ./internal/service/issueflow -run TestRunIssueTrunkRunsHooksOncePerWorkerAttempt
```

Expected before implementation: fail if the current per-turn helper runs hooks multiple times.

- [ ] **Step 2: Make `after_run` best-effort once**

Ensure `after_run` remains logged and ignored on failure, matching `SPEC.md:889-895`. Keep the existing event name `after_run_hook_failed` so old logs/tests remain understandable.

- [ ] **Step 3: Run targeted tests**

Run:

```bash
./test.sh ./internal/service/issueflow ./internal/service/workspace ./internal/service/orchestrator
```

Expected: pass, with hook failures still fatal for `before_run` and non-fatal for `after_run`.

## Task 4: Normalize State Comparisons Everywhere

**Files:**
- Modify: `internal/service/orchestrator/orchestrator.go`
- Modify: `internal/service/issueflow/flow.go`
- Modify: `internal/service/orchestrator/eligibility_test.go`
- Modify: `internal/service/orchestrator/orchestrator_test.go`
- Modify: `internal/service/issueflow/flow_test.go`
- Test: `./test.sh ./internal/service/orchestrator ./internal/service/issueflow`

- [ ] **Step 1: Add case-drift tests**

Add tests proving these inputs behave identically:

```go
activeStates := []string{"in progress"}
terminalStates := []string{"done"}
issueState := "In Progress"
terminalState := "Done"
```

Cover reconciliation and issueflow decision helpers, not only candidate eligibility.

- [ ] **Step 2: Replace exact helper comparisons**

Update exact helpers to use `strings.EqualFold` or a shared normalized-state helper:

```go
func stateNameIn(state string, states []string) bool {
    for _, item := range states {
        if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(state)) {
            return true
        }
    }
    return false
}
```

Use it from `orchestrator.isActive`, `orchestrator.isTerminal`, `issueflow.isActive`, and `issueflow.isTerminal`.

- [ ] **Step 3: Run targeted tests**

Run:

```bash
./test.sh ./internal/service/orchestrator ./internal/service/issueflow
```

Expected: pass, including case-drift tests.

## Task 5: Split Core Scheduler Contract From Issueflow Tracker-Write Extension

**Files:**
- Modify: `docs/contract-scope.md`
- Modify: `docs/runtime-policy.md`
- Modify: `internal/service/issueflow/definition.go`
- Modify: `internal/service/issueflow/flow.go`
- Modify: `internal/service/issueflow/flow_test.go`
- Test: `./test.sh ./internal/service/issueflow ./internal/app`

- [ ] **Step 1: Document the extension boundary precisely**

Update `docs/contract-scope.md` so it says:

```markdown
The SPEC core remains a scheduler/runner and tracker reader. Symphony Go also ships a repo-local `issueflow` extension for this repository's unattended smoke workflow. That extension may perform narrow state writes (`Todo -> In Progress`, `AI Review -> Merging`, `Merging -> Done`) only when enabled by the repo workflow policy and covered by tests. General ticket comments, PR metadata, and arbitrary issue edits remain workflow-agent responsibilities through `linear_graphql`.
```

- [ ] **Step 2: Make extension behavior visible in code**

Add a small policy predicate in `internal/service/issueflow` so state writes are not invisible side effects. The first version can be conservative and derived from existing config:

```go
func stateWriteExtensionEnabled(rt Runtime) bool {
    return rt.Workflow != nil && strings.EqualFold(rt.Workflow.Config.Agent.ReviewPolicy.Mode, "auto")
}
```

Use it to guard `AI Review -> Merging` and `Merging -> Done` writes. Keep `Todo -> In Progress` as a claim/start transition only if `docs/contract-scope.md` and tests explicitly describe it as a scheduler control point.

- [ ] **Step 3: Add tests for disabled extension behavior**

When `agent.review_policy.mode` is not `auto`, assert that `Review: PASS` and `Merge: PASS` do not call `UpdateIssueState`; they should return a wait/continuation outcome that leaves workflow tooling or an operator to move the issue.

- [ ] **Step 4: Run targeted tests**

Run:

```bash
./test.sh ./internal/service/issueflow ./internal/app
```

Expected: pass. Evidence must show extension writes are explicitly policy-gated.

## Task 6: Add Typed Workflow Loader Error Codes And Tighten `$VAR` Semantics

**Files:**
- Modify: `internal/service/workflow/workflow.go`
- Modify: `internal/service/workflow/workflow_test.go`
- Modify: `internal/runtime/config/config.go`
- Modify: `internal/runtime/config/config_test.go`
- Modify: `docs/runtime-policy.md`
- Test: `./test.sh ./internal/service/workflow ./internal/runtime/config ./internal/app`

- [ ] **Step 1: Add workflow error type tests**

Add tests for:

```text
missing_workflow_file
workflow_parse_error
workflow_front_matter_not_a_map
template_parse_error
template_render_error
```

Run:

```bash
./test.sh ./internal/service/workflow -run 'TestLoadReturnsTyped|TestRenderReturnsTyped'
```

Expected before implementation: missing typed loader cases fail.

- [ ] **Step 2: Introduce workflow error codes**

Add a workflow-local error type equivalent to `runtime/config.Error`:

```go
type Error struct {
    Code    string
    Message string
    Err     error
}
```

Expose `Code(err error) string`, and wrap loader/render failures with the spec names.

- [ ] **Step 3: Remove implicit API key fallback**

Change `resolveEnv` so `tracker.api_key` resolves only when set to `$VAR_NAME`. The checked-in workflow already sets `api_key: $LINEAR_API_KEY`, so the default path remains usable.

Run:

```bash
./test.sh ./internal/runtime/config -run TestResolveUsesExplicitEnvIndirectionOnly
```

Expected: pass, and a new test proves empty `tracker.api_key` does not silently read `LINEAR_API_KEY`.

- [ ] **Step 4: Run targeted tests**

Run:

```bash
./test.sh ./internal/service/workflow ./internal/runtime/config ./internal/app
```

Expected: pass.

## Task 7: Add Stable Linear Error Categories

**Files:**
- Modify: `internal/integration/linear/client.go`
- Modify: `internal/integration/linear/client_test.go`
- Test: `./test.sh ./internal/integration/linear`

- [ ] **Step 1: Add failing category tests**

Cover at least:

```text
linear_api_request
linear_api_status
linear_graphql_errors
linear_unknown_payload
linear_missing_end_cursor
```

Run:

```bash
./test.sh ./internal/integration/linear -run TestLinearErrorCategories
```

Expected before implementation: fail for category extraction.

- [ ] **Step 2: Add a typed Linear error**

Implement:

```go
type Error struct {
    Code    string
    Message string
    Err     error
}
```

Use it in request, non-200 status, GraphQL errors, malformed payload, and pagination-integrity failures. Preserve useful raw response snippets only when they cannot contain secrets; never include `Authorization` or token values.

- [ ] **Step 3: Run targeted tests**

Run:

```bash
./test.sh ./internal/integration/linear
```

Expected: pass.

## Task 8: Make Control Plane Reload-Aware And Add SPEC-Compatible HTTP Aliases

**Files:**
- Modify: `internal/app/run.go`
- Modify: `internal/service/control/service.go`
- Modify: `internal/transport/hertzserver/server.go`
- Modify: `internal/transport/hertzserver/server_test.go`
- Test: `./test.sh ./internal/app ./internal/service/control ./internal/transport/hertzserver`

- [ ] **Step 1: Add stale-config regression test**

In an app/control test, simulate workflow reload and assert `RuntimeSettings` reports the reloaded polling interval, workspace root, and active states, not the startup config.

- [ ] **Step 2: Make control service read live state**

Prefer making `control.Service` depend on a small provider interface for current runtime settings/dependencies instead of capturing the initial `Config`, `Workspace`, `Runner`, and `Tracker`. Keep the existing constructor names as wrappers so generated bindings do not need to change.

- [ ] **Step 3: Add compatibility HTTP aliases outside generated routes**

Keep generated POST action routes stable. Add thin non-generated aliases:

```text
GET  /api/v1/state              -> control.GetState
GET  /api/v1/:issue_identifier  -> control.GetIssue
POST /api/v1/refresh            -> control.Refresh
```

Return the same JSON error envelope style as existing Hertz binding tests expect. Preserve the IDL-owned routes from `idl/main.proto:17-69`.

- [ ] **Step 4: Run targeted tests**

Run:

```bash
./test.sh ./internal/app ./internal/service/control ./internal/transport/hertzserver
```

Expected: pass, with both existing generated routes and compatibility aliases working.

## Task 9: Remove Raw Dispatch Printing And Tighten Observability Context

**Files:**
- Modify: `internal/service/orchestrator/orchestrator.go`
- Modify: `internal/service/orchestrator/orchestrator_test.go`
- Modify if needed: `internal/runtime/logging/jsonl.go`
- Test: `./test.sh ./internal/service/orchestrator ./internal/runtime/logging`

- [ ] **Step 1: Replace `fmt.Printf` dispatch output**

Remove direct printing at `internal/service/orchestrator/orchestrator.go:207`. Emit a structured log event instead:

```go
o.logIssue(issue, "dispatch_started", "dispatch started", map[string]any{"state": issue.State})
```

- [ ] **Step 2: Add a logger assertion**

Add or update an orchestrator test so `dispatch_started` includes `issue_id` and `issue_identifier`. This protects `SPEC.md:1257-1267`.

- [ ] **Step 3: Run targeted tests**

Run:

```bash
./test.sh ./internal/service/orchestrator ./internal/runtime/logging
```

Expected: pass.

## Task 10: Final Core Conformance Verification

**Files:**
- Modify: `docs/plan/spec-conformance-ledger.md`
- Validate: `git diff --check`, `./test.sh`, `./build.sh`

- [ ] **Step 1: Update ledger statuses**

After Tasks 2-9 pass, update `docs/plan/spec-conformance-ledger.md`:

```markdown
| 5-6 | workflow_config | covered | typed errors and `$VAR` tests pass | Keep reload smoke in real profile. |
| 7-8 | orchestrator_state | covered | normalized reconciliation and retry tests pass | Real issue smoke remains recommended. |
| 10,12 | agent_runner | covered | one live session continuation test passes | Re-check after Codex protocol upgrades. |
| 13 | observability | covered | structured dispatch log and reload-aware control tests pass | None for core. |
```

- [ ] **Step 2: Run full local verification**

Run:

```bash
git diff --check
./test.sh
./build.sh
```

Expected: all pass. If Go cache permissions fail on this machine, rerun with:

```bash
GOCACHE=/tmp/symphony-go-gocache ./test.sh
GOCACHE=/tmp/symphony-go-gocache ./build.sh
```

- [ ] **Step 3: Run config-only startup check**

Run:

```bash
LINEAR_API_KEY="${LINEAR_API_KEY:-lin_fake_for_config_check}" ./bin/symphony-go run --workflow ./WORKFLOW.md --once --no-tui --issue DOES-NOT-EXIST
```

Expected: startup/config path reaches tracker access. If the key is fake or expired, a Linear auth/transport failure is acceptable for this local config-only check and must be reported as environment-dependent, not as a passed real integration.

## Execution Order

1. Task 1 first, because it gives future agents a small source of truth.
2. Tasks 2 and 3 together, because session reuse and hook scope are coupled.
3. Task 4 before broader smoke, because state normalization affects dispatch, retry, and reconciliation.
4. Task 5 after continuation is stable, because issueflow state writes are easier to reason about once the worker session boundary is correct.
5. Tasks 6 and 7 after behavior is stable, because they mostly harden error surfaces.
6. Tasks 8 and 9 last before final verification, because they affect extension/operator surfaces rather than core dispatch semantics.

## Risk Controls

- Keep all behavior changes behind package-level tests before running full scripts.
- Do not modify `gen/hertz/...` by hand; if route generation changes become necessary, use the existing IDL generation workflow documented in `docs/control-plane-hertz-idl.md`.
- Treat Linear writes as the highest-risk behavior. Each task that changes state-write behavior must use fakes first, then a real isolated issue only under the Real Integration Profile.
- Preserve current checked-in `WORKFLOW.md` semantics unless a task explicitly changes and validates the issueflow extension boundary.
- Do not run real issue smoke with fake or expired `LINEAR_API_KEY`; report it as skipped or blocked.

## Definition Of Done

- The conformance ledger exists and maps each relevant `SPEC.md` range to current evidence.
- Worker continuation uses one live app-server session/thread per worker attempt.
- `before_run` and `after_run` execute once per worker attempt.
- All issue-state comparisons used for dispatch, reconciliation, and issueflow are normalized.
- Workflow loader and Linear client expose stable error categories.
- `tracker.api_key` follows explicit `$VAR` semantics.
- Issueflow tracker writes are either policy-gated as a documented extension or removed from core behavior.
- HTTP extension either has SPEC-compatible aliases or clearly documented extension-only divergence.
- `git diff --check`, targeted package tests, `./test.sh`, and `./build.sh` pass or have an explicit environment-blocked reason.
