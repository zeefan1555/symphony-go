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

Current orchestrator-owned tracker writes are limited to runtime control points such as claiming work, same-session review and merge gates, and final terminal cleanup. General ticket writing remains workflow-owned through the injected `linear_graphql` tool.

## Optional Surfaces

The terminal TUI and loopback HTTP control plane are operator surfaces around the core service. They do not change the scheduler/runner contract and are not a rich web UI or multi-tenant control plane.

The Linear client exposes write helpers because the current implementation ships orchestrator gates, the optional `linear_graphql` tool, and diagnostic control routes. These helpers are not a separate business workflow engine.
