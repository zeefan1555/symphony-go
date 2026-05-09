# Runtime Policy

This document records Symphony Go's implementation-defined runtime policy for `SPEC.md`.

## Trust Boundary

Symphony Go is a high-trust local automation runner. It is intended for trusted repositories, trusted workflow files, and issue sources that operators are willing to let a local coding agent execute inside per-issue worktrees.

The service isolates normal implementation work to the issue workspace and git metadata roots needed by local worktrees, but it does not claim to provide a strong security sandbox beyond the configured Codex app-server policy, host OS permissions, and repository workflow rules.

## Approval And Sandbox Policy

Runtime policy is loaded from `WORKFLOW.md`:

- `codex.approval_policy` is forwarded to app-server `thread/start` and `turn/start`.
- `codex.thread_sandbox` is forwarded to app-server `thread/start`.
- `codex.turn_sandbox_policy` is forwarded to app-server `turn/start`.

Current repository workflow sets `codex.approval_policy: never` and `codex.thread_sandbox: workspace-write`. The runner augments `workspaceWrite` turn writable roots with the current issue workspace and git metadata roots discovered from that workspace.

Current repository workflow sets `codex.turn_sandbox_policy.type: workspaceWrite`, full read-only access, network access enabled, and no `/tmp` or `TMPDIR` exclusion. The implementation forwards that policy to Codex app-server and only adds the per-issue writable roots needed for local worktree execution.

Operator-confirmation posture is fail-fast for unattended child sessions. Symphony Go does not pause a child run for interactive approval or user input; targeted approval and input events fail the attempt so the orchestrator can retry, surface a blocker, or continue according to workflow policy.

Operators should tighten these fields before using untrusted issue sources, untrusted workflow edits, or repositories whose hooks should not run with local developer credentials.

## Secret Handling

Workflow config supports explicit `$VAR` indirection for secrets such as `tracker.api_key`. Symphony Go validates the presence of required secrets, but operator-facing runtime settings and control-plane projections must not include API tokens or resolved secret values.

Errors may name the missing configuration field or expected environment variable, but must not print the secret value itself. Logs and workpads should refer to credentials by field name, environment variable name, or blocker category.

## Hook Safety

Workspace hooks are trusted shell scripts from `WORKFLOW.md`. They run with the per-issue workspace as cwd and use the configured hook timeout so a stuck hook does not block the orchestrator indefinitely.

Hook command, output, and error strings are logged only as shortened previews. Operators should avoid printing secrets from hooks because even truncated output is still operator-visible.

## User Input And Approval Requests

Unattended runs must not wait indefinitely for human input. Symphony Go therefore treats targeted app-server user-input or approval request events as worker failures. The orchestrator then handles the failed attempt through its normal retry path.

The runner currently fails fast for these app-server events:

- `turn/input_required`
- `turn/approval_required`
- `item/tool/requestUserInput`
- `item/commandExecution/requestApproval`
- `item/fileChange/requestApproval`
- `mcpServer/elicitation/request`

This policy keeps child sessions from silently hanging on approval prompts. It also enforces the workflow rule that unattended child sessions should use the injected `linear_graphql` tool for Linear writes instead of Linear MCP/app writes that may trigger interactive approvals.

## Dynamic Tools

Symphony Go implements the optional `linear_graphql` client-side tool. The tool is advertised to app-server sessions through `thread/start.dynamicTools` when Linear auth is configured, and each tool call executes exactly one GraphQL operation through the active workflow's Linear endpoint and token.

Invalid input, unsupported tool names, missing auth, transport failures, and GraphQL top-level `errors` all return `success=false` tool output that the model can inspect in-session.

## Run Attempt Lifecycle And Error Categories

Symphony Go exposes run attempt state as an observable split-state projection rather than a persisted `RunAttempt` database record. The current projection is `observability.RunningEntry` while work is active and `observability.RetryEntry` when an attempt is waiting for retry. Together these records carry the issue id, issue identifier, tracker state, agent phase, stage, workspace path, attempt number, session/thread/turn ids, pid, token counters, runtime seconds, last event, last message, and retry error.

Lifecycle stages are normalized for operator visibility around the actions the service owns:

- `queued`: the orchestrator claimed an issue, removed any stale retry entry, and queued a worker.
- `preparing_workspace`: the workspace manager is resolving, validating, creating, or reusing the per-issue workspace.
- `running_workspace_hooks`: configured workspace hooks are running.
- `rendering_prompt`: the workflow prompt is being rendered for the current attempt.
- `running_agent`: the Codex app-server turn is active for the implementation phase.
- `continuing_ai_review`, `continuing_merging`, and `continuing_implementation`: the same live session is continuing in the workflow-defined next phase.

Terminal behavior is represented by `issueflow.Result.Outcome` plus the worker error and retry entry:

- `done` and `wait_human` release the claim without retry.
- `stopped` releases the claim when the current issue state is no longer active, the context is canceled, or the run has reached its workflow-defined stop point.
- `retry_continuation` schedules the short continuation retry for an active issue after a normal turn.
- `retry_failure` schedules the failure backoff path.
- A non-nil worker error is logged and schedules the same failure backoff path.

Codex runner failures are not a separate public enum. Operators should treat the stable categories as the message prefixes and retry path currently emitted by the runner: app-server startup/read timeout, protocol response error, turn timeout, turn failed, turn cancelled, approval required, user input required, stream EOF, and subprocess wait failure. The orchestrator maps these to retryable worker failure unless the issue state or workflow outcome says to stop.

## Issueflow State Writes

The core runtime reads tracker state, schedules workers, and runs one live Codex app-server session per worker attempt. The repo-local `issueflow` extension performs narrow state writes for unattended smoke workflow control: claiming `Todo -> In Progress`, advancing `AI Review -> Merging`, and marking `Merging -> Done`. Review and merge advancement are enabled only when `agent.review_policy.mode: auto`; other modes leave the issue for workflow tooling or an operator.

## Harness Hardening

The current repository workflow is optimized for a trusted local development harness. Before using untrusted tracker data, untrusted workflow edits, or broader credentials, operators should reduce the available credentials, tool surface, filesystem writable roots, and network access to the minimum required for that workflow.

The injected `linear_graphql` tool uses the configured Linear credential. Treat that credential as the enforcement boundary for project, team, and workspace access; use the narrowest practical token and tracker configuration.
