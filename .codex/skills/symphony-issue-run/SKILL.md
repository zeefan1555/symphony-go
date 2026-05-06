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

- Default flow is fully automated: `Todo -> In Progress -> AI Review -> Merging -> Done`.
- The listener, workflow, and orchestrator own state transitions; do not pause
  for a human to move `Human Review` or `Merging` unless the user explicitly asks
  for a manual gate.
- `Human Review` is only an exceptional hold for real external blockers such as
  missing auth, unavailable tools, or permissions that cannot be fixed in-session.
- Linear reads and writes in child agents must follow `WORKFLOW.md`: use the
  injected `linear_graphql` path, matching the listener's Linear GraphQL client.
  Do not use Linear MCP/app issue or comment writes in unattended child sessions.
- Issue work happens in `.worktrees/<ISSUE>`. Post-run optimization of the repo
  happens in the repo root after the target issue is terminal.
- Every observed improvement must be recorded in
  `docs/optimization/symphony-issue-run.md` before committing.
- After verified optimization changes, create a local commit on `main` and push
  `origin/main`.

## Preflight

Run from the repository root and confirm branch, workflow policy, and Linear
GraphQL automation contract:

```sh
git rev-parse --show-toplevel
pwd
git status --short --branch
rg -n "review_policy:|mode:|on_ai_fail|active_states|terminal_states" WORKFLOW.md
```

Confirm the target project/team and any created smoke issue through the
supervising session's Linear tools. Do not treat `linear --version`,
`linear auth whoami`, or Linear MCP approval as the health gate for the child
automation path.

Read `WORKFLOW.md` before creating the issue. The workflow is the source of
truth for active states, AI review policy, worktree root, merge behavior, and
the model command. Model changes belong in `codex.command`; this skill should
not hardcode a model assumption.

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
- 使用当前仓库的 `.worktrees/<ISSUE>` worktree。
- 让 Symphony Go listener 按 `WORKFLOW.md` 全自动跑完：
  `Todo -> In Progress -> AI Review -> Merging -> Done`。
- Linear 读写必须使用派生会话可用的 `linear_graphql` 工具；不要调用
  Linear MCP/app issue/comment 写入或 `linear` CLI 兜底。
- Linear workpad、状态说明、commit message 和可见说明默认使用中文。
- `Merging` 阶段使用 `.codex/skills/pr/SKILL.md` 的 PR merge flow；
  不在当前 sandbox 内直接把 issue worktree 分支合入本地 `main`。
```

Capture the identifier and URL, then verify identifier, URL, state, team, and
project through Linear GraphQL or the supervising session's Linear tooling.

Use `Backlog` only when the user explicitly wants a manual start. For the normal
automation loop, create the issue directly in `Todo`.

## Start One Issue-Scoped Listener

First check whether a listener is already running. Do not start a duplicate
service just because an old PID file exists:

```sh
pgrep -fl 'bin/symphony-go run --workflow ./WORKFLOW.md' || true
test -f .symphony/pids/symphony-go.pid && cat .symphony/pids/symphony-go.pid || true
test -f ".symphony/pids/<ISSUE>.pid" && cat ".symphony/pids/<ISSUE>.pid" || true
```

For the default lifecycle, start one issue-scoped listener from the repo root and
let it run until the target issue reaches `Done` or another terminal state:

```sh
ISSUE=<ISSUE>
make build
mkdir -p .symphony/logs .symphony/pids
ts=$(date +%Y%m%d-%H%M%S)
log=".symphony/logs/${ISSUE}-$ts.out"
python3 - "$ISSUE" "$log" <<'PY'
import subprocess
import sys

issue, log = sys.argv[1], sys.argv[2]
f = open(log, "ab", buffering=0)
cmd = [
    "./bin/symphony-go",
    "run",
    "--workflow",
    "./WORKFLOW.md",
    "--no-tui",
    "--issue",
    issue,
    "--merge-target",
    "main",
]
p = subprocess.Popen(
    cmd,
    stdin=subprocess.DEVNULL,
    stdout=f,
    stderr=subprocess.STDOUT,
    start_new_session=True,
)
pid_file = f".symphony/pids/{issue}.pid"
with open(pid_file, "w") as out:
    out.write(str(p.pid))
print(f"pid={p.pid} log={log} pid_file={pid_file}")
PY
```

This detached Python launcher is preferred over plain `nohup ... &`; previous
smoke runs showed `nohup` could exit immediately with an empty daemon log.

Use `run-once` only for diagnosis, not for the normal full automation loop:

```sh
ISSUE=<ISSUE> WORKFLOW=./WORKFLOW.md MERGE_TARGET=main make run-once
```

## Monitor To Terminal

Watch the issue, human log, and daemon log until terminal. Do not stop at
`AI Review`, `Rework`, or `Merging`.

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
- `.worktrees/<ISSUE>` is created.
- Workpad receives `initial`, `handoff`, `ai_review`, and merge/terminal evidence.
- AI Review passes or sends the issue to `Rework` with reasons.
- Rework produces a new implementation commit and returns to `AI Review`.
- `Merging` uses `.codex/skills/pr/SKILL.md` to create/update a GitHub PR,
  wait for checks, squash-merge it, and sync root `main`.
- Issue reaches `Done`.

If the issue stalls, diagnose before restarting:

```sh
git worktree list --porcelain | rg -n "$ISSUE|\\.worktrees/$ISSUE" || true
rg -n "$ISSUE|state_changed|ai_review|rework|merge|blocked|error" "$latest_human"
rg -n "$ISSUE|state_changed|ai_review|rework|merge|blocked|error" .symphony/logs/run-*.jsonl
ps -ef | rg 'symphony-go|codex app-server' | rg -v rg
```

Also re-read the issue before restarting.

Treat repeated stalls as framework signal. Do not keep restarting blindly.

## Merging Checks

`Merging` must use the PR merge flow documented in `.codex/skills/pr/SKILL.md`.
The issue worktree branch is pushed, a GitHub PR is created or updated, checks
are handled there, and the PR is squash-merged before root `main` is synced.

Do not directly run `git merge --no-ff <issue-branch>` in the root checkout for
the normal path. If the PR flow fails, inspect the exact PR/script/GitHub
blocker and record it instead of falling back to local main merge.

After terminal state, verify:

```sh
git status --short --branch
git rev-parse --short HEAD
git rev-parse --short origin/main
git worktree list --porcelain | rg -n "<ISSUE>|\\.worktrees/<ISSUE>" || true
```

Use Linear GraphQL or the supervising session's Linear tooling to verify final
issue state, workpad evidence, and links.

## Stop The Issue Listener

Stop only the issue-scoped listener after the target issue is terminal. Keep a
long-lived all-issue listener alive only if the user explicitly asked for it.

```sh
pid_file=.symphony/pids/<ISSUE>.pid
if [ -f "$pid_file" ]; then
  kill "$(cat "$pid_file")" 2>/dev/null || true
  rm "$pid_file"
fi
pgrep -fl 'symphony-go run --workflow ./WORKFLOW.md|bin/symphony-go run' || true
```

## Review The Run

After terminal state and listener cleanup, always review the run before
reporting back. Use the latest human log, JSONL log, Linear workpad, git status,
and worktree cleanup evidence.

Classify findings:

- Skill gaps: instructions caused manual waiting, duplicate listeners, wrong
  team/project/state, missing terminal checks, or unclear handoff.
- Workflow gaps: prompt caused unnecessary remote pulls, wrong PR flow usage, wrong state
  routing, weak AI Review/Rework loop, or ambiguous repo-root/worktree behavior.
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
rg -n "StateSkills|runStateSkill|agent\\.state_skills" internal WORKFLOW.md README.md docs || true
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

- Do not stop at `Human Review`; default flow should not go there.
- Do not stop at `AI Review` or `Merging`; wait for terminal or a real blocker.
- Do not run only `make run-once` when the user asked for a full framework run.
- Do not start a second listener when one is already polling the same issue.
- Do not let child agents fall back to Linear CLI or Linear MCP/app writes when
  the workflow requires `linear_graphql`.
- Do not hardcode another repository path, remote, branch, project, or model.
- Do not declare health from a PID file alone; confirm with `ps` and logs.
- Do not delete `.worktrees/<ISSUE>` manually while the issue is active.
- Do not declare `Merging` complete until root checkout, `origin/main`, Linear
  state, workpad evidence, and worktree cleanup have all been checked.
- Do not leave optimization learnings only in chat; record them in the docs,
  then commit and push verified changes.

## Handoff

Report back with:

- Linear issue identifier and URL.
- Listener mode, PID, daemon log, and human/JSONL log paths.
- Final issue state.
- Worktree path and cleanup result.
- AI Review result and any Rework loop.
- PR URL, squash merge/root sync evidence if `Merging` ran.
- Root checkout status after merge.
- Optimization notes recorded, files changed, validation commands, commit, and
  push result.
