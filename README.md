# Go Symphony

Go Symphony is a minimal Go implementation of the local Symphony workflow.

It reads `go/WORKFLOW.md`, polls Linear, creates one local worktree per issue under `go/.worktrees/`, runs Codex through `codex app-server`, and uses the local review / merge flow described in the workflow prompt.

## Quick Start

Run tests first:

```bash
cd /Users/bytedance/symphony/go
make test
```

Build the local binary:

```bash
cd /Users/bytedance/symphony/go
make build
```

## Start the TUI daemon

Start the daemon for continuous issue monitoring with the terminal dashboard:

```bash
cd /Users/bytedance/symphony/go
make run
```

This keeps polling the configured Linear project in `WORKFLOW.md`, dispatches matching active issues, and renders the `SYMPHONY STATUS` dashboard in the terminal.

To run continuously without the dashboard:

```bash
cd /Users/bytedance/symphony/go
bin/symphony-go run --workflow ./WORKFLOW.md --no-tui
```

For local debugging only, run a single poll for one issue:

```bash
cd /Users/bytedance/symphony/go
make run-once ISSUE=ZEE-8
```

`make run-once` uses `--once --no-tui`, so it is useful for checking Linear connectivity and config without taking over the terminal UI.

The `Makefile` intentionally builds `bin/symphony-go` before running it. On some macOS setups, `go run` can fail while loading the temporary executable with:

```text
missing LC_UUID load command
```

Using `make build`, `make run`, or `make run-once` avoids that temporary executable path.

If you still want to run without the Makefile, prefer:

```bash
go run -ldflags=-linkmode=external ./cmd/symphony-go run --workflow ./WORKFLOW.md --once --issue ZEE-8
```

## TUI Fields

The terminal dashboard shows:

- `Agents`: running agents against the configured max concurrency.
- `Throughput`: current observed turn activity.
- `Runtime`: aggregate finished runtime plus active session runtime.
- `Tokens`: Codex input/output/total token totals when available.
- `Project`: Linear project URL derived from `WORKFLOW.md`.
- `Next refresh`: current polling status or next poll countdown.
- `Running`: active issue sessions, state, compact session id, age/turn, tokens, and latest event.
- `Backoff queue`: issues waiting for retry.

## V3 Runtime Contract

The Go daemon is a high-trust local Symphony runner. It reads `WORKFLOW.md`, polls Linear, creates one sanitized workspace per issue, launches Codex only from that issue workspace, and preserves workspaces across successful runs.

`WORKFLOW.md` changes are reloaded without restart. Invalid edits do not replace the last known good workflow; the daemon logs the reload error and keeps polling, retry, and reconciliation alive.

Runtime ownership stays in the daemon. It claims issues before dispatch, enforces global and per-state concurrency, tracks running sessions in the snapshot, and updates the persistent `## Codex Workpad` comment after local commits.

## Trust And Safety Posture

This implementation is intended for trusted local development environments. The default Codex approval policy is `never`, the default thread sandbox is `workspace-write`, and the turn sandbox includes the issue workspace plus git metadata roots needed by local worktrees. Operators should tighten these values in `WORKFLOW.md` before using untrusted issue sources.

Secrets are supplied by explicit `$VAR` references or `LINEAR_API_KEY`. The daemon validates secret presence without logging secret values.

Workspace paths are sanitized and must stay under the configured workspace root. Worker launch is rejected if the resolved workspace path escapes that root or is not the expected per-issue workspace.

## Retry And Reconciliation

Normal worker exits schedule a 1 second continuation retry so Symphony can re-check whether the issue is still active. Worker failures use exponential backoff starting at 10 seconds and capped by `agent.max_retry_backoff_ms`.

Every poll reconciles running issues against Linear. Terminal states cancel the worker and remove the workspace. Non-active non-terminal states cancel the worker and preserve the workspace.

Startup cleanup also asks Linear for terminal issues and removes matching local workspaces. Cleanup failures are logged without aborting daemon startup.

## Validation

Use the Makefile path for routine validation:

```bash
cd /Users/bytedance/symphony/go
make test
make build
```

For targeted package work, run the smallest relevant `go test` command first, then finish with `make test` and `make build` before declaring the daemon ready.

## Workflow File

The Go version uses:

```text
/Users/bytedance/symphony/go/WORKFLOW.md
```

The workspace root in that file is relative to the workflow file directory, so:

```yaml
workspace:
  root: .worktrees
```

means:

```text
/Users/bytedance/symphony/go/.worktrees
```

The Go workflow owns its hook scripts inside the Go module:

```text
/Users/bytedance/symphony/go/scripts/symphony_after_create.sh
/Users/bytedance/symphony/go/scripts/symphony_before_remove.sh
```

The Go after-create hook always creates `symphony-go/<issue>` branches. It does not depend on `elixir/WORKFLOW.md` or the root `scripts/` directory.

## State Flow

For the local smoke workflow:

```text
Todo -> In Progress -> Human Review -> Merging -> Done
```

During `Merging`, the Go orchestrator merges the issue branch, for example `symphony-go/ZEE-8`, into the configured local target branch. The Go workflow uses the `symphony-go/` branch prefix so it does not collide with Elixir smoke worktrees such as `symphony/ZEE-8`. The default target branch is:

```text
feat_zff
```

Override it when needed:

```bash
make run MERGE_TARGET=<branch>
```

## UTF-8

The Go version is expected to support Chinese from v1:

- Workflow prompt text is rendered as UTF-8.
- Linear GraphQL requests use `encoding/json`.
- GraphQL requests send `Content-Type: application/json; charset=utf-8`.
- Hooks and Codex child processes receive UTF-8 locale defaults.
- JSONL logs keep UTF-8 text intact.

## Logs

Runtime logs are written under:

```text
/Users/bytedance/symphony/go/.symphony/logs/
```
