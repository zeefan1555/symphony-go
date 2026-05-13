---
name: symphony-issue-run
description: Use when this Symphony Go repo needs a Linear issue run, end-to-end listener execution, run review, or workflow/skill/code optimization from observed blockers.
---

# Symphony Issue Run

## Scope

This skill is scoped to `/Users/bytedance/symphony-go`.

Use it when the user wants an agent to create a Linear issue for this repo,
start the local Symphony Go listener, let the framework run the issue to a
terminal state, review the run, and fold concrete improvements back into the
repo.

Do not use it for other repositories.

## Operating Contract

- Default flow uses the configured target branch directly:
  `Todo -> In Progress -> AI Review -> Pushing -> Done`.
- The listener, workflow, and orchestrator own state transitions; do not pause
  for a human after `AI Review` unless the user explicitly asks for a manual
  gate.
- `Human Review` is only an exceptional hold for real external blockers such as
  missing auth, unavailable tools, or permissions that cannot be fixed in-session.
- Linear reads and writes in child agents must follow `workflows/WORKFLOW-symphony-go.md`: use the
  injected `linear_graphql` path, matching the listener's Linear GraphQL client.
  Do not use Linear MCP/app issue or comment writes in unattended child sessions.
- Issue work happens in the repo root on the configured target branch. Do not
  create issue worktrees, PR branches, scratch checkouts, or temporary clones.
- Every observed improvement must be recorded in
  `docs/optimization/symphony-issue-run.md` before committing.
- After verified optimization changes, create a local commit on the configured
  target branch and push it to origin.

## Preflight

Run from the repository root and confirm branch, workflow policy, and Linear
GraphQL automation contract:

```sh
git rev-parse --show-toplevel
pwd
git status --short --branch
rg -n "review_policy:|mode:|on_ai_fail|active_states|terminal_states" workflows/WORKFLOW-symphony-go.md
```

Confirm the target project/team and any created smoke issue through the
supervising session's Linear tools. Do not treat `linear --version`,
`linear auth whoami`, or Linear MCP approval as the health gate for the child
automation path.

Read `workflows/WORKFLOW-symphony-go.md` before creating the issue. The workflow is the source of
truth for active states, AI review policy, static repo-root cwd, target branch
push behavior, and the model command. Model changes belong in `codex.command`;
this skill should not hardcode a model assumption.

If `git status --short` shows unrelated local edits, do not overwrite them.
Either finish the current repo optimization first or stop and report the
conflict.

## Create The Issue

Create the issue from the supervising session, then let the child workflow use
`linear_graphql` for Linear reads/writes. The issue body should follow this
shape:

```md
## 背景
<写清楚为什么要做>

## 任务
- <可执行步骤 1>
- <可执行步骤 2>

## 验证
- <必须执行的验证命令或检查>

## 约束
- 只在 `/Users/bytedance/symphony-go` 仓库内工作。
- 使用当前仓库 repo root 和配置的目标分支，不创建 worktree、临时 clone 或 PR 分支。
- 让 Symphony Go listener 按 `workflows/WORKFLOW-symphony-go.md` 全自动跑完：
  `Todo -> In Progress -> AI Review -> Pushing -> Done`。
- Linear 读写必须使用派生会话可用的 `linear_graphql` 工具；不要调用
  Linear MCP/app issue/comment 写入或 `linear` CLI 兜底。
- Linear workpad、状态说明、commit message 和可见说明默认使用中文。
- `AI Review` 只审查本地 commit、workpad 和 validation evidence；不要创建 PR。
- `AI Review` 通过后以 `Review: PASS` 交给框架进入 `Pushing`。
- `Pushing` 阶段执行 `git push origin <target>`，更新 workpad push evidence，并以 `Push: PASS` 结束。
```

Capture the identifier and URL, then verify identifier, URL, state, team, and
project through Linear GraphQL or the supervising session's Linear tooling.

Use `Backlog` only when the user explicitly wants a manual start. For the normal
automation loop, create the issue directly in `Todo`.

## Start One Issue-Scoped Listener

First check whether a listener is already running. Do not start a duplicate
service just because an old PID file exists:

```sh
pgrep -fl 'bin/symphony-go run --workflow ./workflows/WORKFLOW-symphony-go.md' || true
test -f .symphony/pids/symphony-go.pid && cat .symphony/pids/symphony-go.pid || true
test -f ".symphony/pids/<ISSUE>.pid" && cat ".symphony/pids/<ISSUE>.pid" || true
```

For the default single-issue lifecycle, run one issue-scoped foreground pass from
the repo root. `--once` waits for the dispatched worker to finish, so the command
should exit after the target issue reaches `Done` or another terminal state:

```sh
ISSUE=<ISSUE>
make build
date '+SMOKE_START %Y-%m-%dT%H:%M:%S%z'
./bin/symphony-go run --workflow ./workflows/WORKFLOW-symphony-go.md --once --no-tui --issue "$ISSUE" --merge-target main
date '+SMOKE_END %Y-%m-%dT%H:%M:%S%z'
```

Use a long-lived listener only when the user explicitly asks to process multiple
issues continuously. Do not start a duplicate listener for a single smoke run.

## Monitor To Terminal

Watch the issue, human log, and daemon log until terminal. Do not stop at
`AI Review`, `Pushing`, or `Rework`.

```sh
ISSUE=<ISSUE>
latest_human=$(ls -t .symphony/logs/run-*.human.log | head -1)
tail -n 160 "$latest_human"
tail -n 120 "$log"
```

Use Linear GraphQL or the supervising session's Linear tooling to verify the
current issue state while monitoring.

Expected healthy evidence:

- Issue moves `Todo -> In Progress`.
- Worker cwd is the repo root on the configured target branch.
- Workpad receives initial, handoff, AI Review, and push evidence.
- Implementation creates one or more local commits before `AI Review`.
- AI Review either reports `Review: PASS` and enters `Pushing`, or sends the
  issue to `Rework` with reasons.
- Rework produces a new implementation commit and returns to `AI Review`.
- Pushing pushes the target branch, records push evidence, and reports `Push: PASS`.
- Issue reaches `Done`.

If the issue stalls, diagnose before restarting:

```sh
rg -n "$ISSUE|state_changed|ai_review|pushing|rework|push|blocked|error" "$latest_human"
rg -n "$ISSUE|state_changed|ai_review|pushing|rework|push|blocked|error" .symphony/logs/run-*.jsonl
ps -ef | rg 'symphony-go|codex app-server' | rg -v rg
```

Also re-read the issue before restarting.

Treat repeated stalls as framework signal. Do not keep restarting blindly.

## Push Checks

This workflow does not use PR-based `Merging`. After `AI Review` passes, the
framework moves the issue to `Pushing`; in `Pushing`, the agent pushes the
configured target branch and reports `Push: PASS` so the framework can mark
`Done`.

Before declaring success, confirm:

- The current branch is the configured target branch.
- The issue is in `Pushing`.
- `git status --short` is clean.
- The relevant local commit range and validation evidence are recorded in the
  workpad.
- `git push origin <target>` succeeded.
- Workpad push evidence records branch, pushed commit, validation summary, and
  AI Review verdict.

After terminal state, verify:

```sh
git status --short --branch
git rev-parse --short HEAD
git rev-parse --short origin/<target>
```

Use Linear GraphQL or the supervising session's Linear tooling to verify final
issue state and workpad evidence.

## Stop The Issue Listener

For the default `--once` smoke command there should be no listener to stop after
the command exits. If a long-lived or detached listener was explicitly started,
stop only that issue-scoped listener after the target issue is terminal. Keep a
long-lived all-issue listener alive only if the user explicitly asked for it.

```sh
pid_file=.symphony/pids/<ISSUE>.pid
if [ -f "$pid_file" ]; then
  kill "$(cat "$pid_file")" 2>/dev/null || true
  rm "$pid_file"
fi
pgrep -fl 'symphony-go run --workflow ./workflows/WORKFLOW-symphony-go.md|bin/symphony-go run' || true
```

## Review The Run

After terminal state and listener cleanup, always review the run before
reporting back. Use the latest human log, JSONL log, Linear workpad, git status,
and push evidence.

Classify findings:

- Skill gaps: instructions caused manual waiting, duplicate listeners, wrong
  team/project/state, missing terminal checks, or unclear handoff.
- Workflow gaps: prompt caused PR flow fallback, wrong state routing, weak
  AI Review/Rework loop, unsafe target-branch handling, or ambiguous repo-root
  behavior.
- Code gaps: behavior should be first-class in the orchestrator, workspace
  manager, Linear adapter, runner, logs, or TUI instead of relying on humans.
- Environment gaps: auth, git permissions, stale processes, or local filesystem
  residue blocked automation.

Optimize only when the root cause is clear and the change is small enough to
verify immediately. Otherwise create a follow-up Linear issue in `Backlog` with
file anchors, reproduction evidence, and acceptance criteria.

## Record Optimization Notes

Every retained optimization must be written to
`docs/optimization/symphony-issue-run.md` before commit. Append an entry with:

- Date/time and issue identifier.
- What blocked or slowed the run.
- Evidence: log path, workpad state, git commit, command output, or file anchor.
- Decision: Skill, Workflow, code, or follow-up issue.
- Files changed and validation commands.

Use this template:

```md
## YYYY-MM-DD HH:MM - <ISSUE or repo-only>

- Trigger:
- Evidence:
- Optimization:
- Files:
- Validation:
- Follow-up:
```

## Commit And Push Optimizations

After updating the skill, workflow, docs, or code, verify and publish from the
repo root:

```sh
git status --short --branch
rg -n "StateSkills|runStateSkill|agent\\.state_skills" internal workflows/WORKFLOW-symphony-go.md README.md docs || true
git diff --check
go test ./internal/config ./internal/orchestrator ./internal/workflow ./internal/types
CGO_ENABLED=0 go test ./...
git status --short --branch
git add <changed files>
git commit -m "<concise message>"
git push origin main
```

If plain `go test ./cmd/symphony-go` fails with local `dyld missing LC_UUID load
command`, record that environment-specific failure and use `CGO_ENABLED=0 go
test ./...` as the publish gate.

Do not commit unrelated local edits. If unrelated edits exist, stop and report
the exact files.

## Common Mistakes

- Do not stop at `Human Review`; default flow should only use it for true external blockers.
- Do not stop at `AI Review` or `Pushing`; wait for terminal or a real blocker.
- Do not start a long-lived listener when the user asked for one issue smoke;
  use `./bin/symphony-go run --workflow ./workflows/WORKFLOW-symphony-go.md --once --no-tui --issue <ISSUE> --merge-target main`.
- Do not start a second listener when one is already polling the same issue.
- Do not let child agents fall back to Linear CLI or Linear MCP/app writes when
  the workflow requires `linear_graphql`.
- Do not hardcode another repository path, remote, branch, project, or model.
- Do not declare health from a PID file alone; confirm with `ps` and logs.
- Do not create or delete `.worktrees/<ISSUE>` for this workflow.
- Do not create PRs or run `pr_merge_flow.sh`; success is `Pushing` push
  evidence plus terminal issue state.
- Do not leave optimization learnings only in chat; record them in the docs,
  then commit and push verified changes.

## Handoff

Report back with:

- Linear issue identifier and URL.
- Listener mode, PID, daemon log, and human/JSONL log paths.
- Final issue state.
- Repo root, target branch, and pushed commit.
- AI Review result and any Rework loop.
- Push evidence and root checkout status.
- Optimization notes recorded, files changed, validation commands, commit, and
  push result.
