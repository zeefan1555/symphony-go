# Contract Scope

This document records the implementation boundary for the SPEC sections on problem statement, goals, non-goals, and system overview.

## Core Service Boundary

Symphony Go is a long-running automation service. Its core job is to read eligible tracker work, create or reuse an isolated per-issue workspace, run a Codex app-server session in that workspace, reconcile tracker state, retry when appropriate, and emit operator-visible logs.

The core runtime is assembled from these SPEC components:

- Workflow Loader and Config Layer: `internal/service/workflow` and `internal/runtime/config`.
- Issue Tracker Client: `internal/integration/linear`.
- Orchestrator: `internal/service/orchestrator`.
- Workspace Manager: `internal/service/workspace`.
- Agent Runner: `internal/service/codex`.
- Logging and status snapshots: `internal/runtime/logging` and `internal/runtime/observability`.

## Policy Boundary

Repository policy stays in `WORKFLOW.md`. Ticket editing rules, PR handling, validation gates, workpad shape, and handoff wording belong in the workflow prompt and agent skills unless a SPEC-required orchestrator gate needs a narrow state transition.

The SPEC core remains a scheduler/runner and tracker reader. Symphony Go also ships a repo-local `issueflow` extension for this repository's unattended smoke workflow. That extension may perform narrow state writes (`Todo -> In Progress`, `AI Review -> Merging`, `Merging -> Done`) only when enabled by the repo workflow policy and covered by tests. General ticket comments, PR metadata, and arbitrary issue edits remain workflow-agent responsibilities through `linear_graphql`.

Run completion is workflow-defined. The core service does not require every successful agent run to push an issue all the way to `Done`; a run may stop at a workflow handoff state such as `Human Review`, `AI Review`, or `Merging` when the repository workflow defines that as the correct operator or agent boundary.

## Optional Surfaces

The terminal TUI and loopback HTTP control plane are operator surfaces around the core service. They do not change the scheduler/runner contract and are not a rich web UI or multi-tenant control plane.

Symphony Go is not a general-purpose workflow engine or distributed job scheduler. Workflow branching, product-specific ticket edits, PR handling, and human handoff rules stay in `WORKFLOW.md`, agent skills, and narrow repo-local extensions instead of becoming a generic orchestration DSL.

The Linear client exposes write helpers because the current implementation ships orchestrator gates, the optional `linear_graphql` tool, and diagnostic control routes. These helpers are not a separate business workflow engine.
