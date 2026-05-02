---
name: symphony-issue-run
description: Use when operating this Symphony Go repository by creating a Linear issue and starting the local listener to process it.
---

# Symphony Issue Run

## Scope

This skill is scoped to `/Users/bytedance/symphony-go`.

Use it when the user wants an agent to create a Linear issue for this repo and
then start the local Symphony Go listener so the workflow handles that issue.
Do not use it for other repositories.

## Preconditions

- Run from the repository root:

```sh
git rev-parse --show-toplevel
pwd
```

- Confirm tools and auth:

```sh
linear --version
linear auth whoami
git status --short --branch
```

- Read `WORKFLOW.md` before creating the issue. The current workflow defines
  the Linear project, active states, workspace root, review policy, and main
  merge flow.

## Create The Issue

Create the issue with `linear` CLI. Do not use Linear MCP/app tools for this
workflow; unattended MCP writes can trigger approval prompts.

Use a temporary Markdown file for the body:

```sh
tmp_issue=$(mktemp)
cat > "$tmp_issue" <<'EOF'
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
- Linear workpad 和可见说明默认使用中文。
- 不创建 PR；当前 workflow 在 `Merging` 阶段把本地 issue worktree
  分支合入本地 `main`，验证后 `git push origin main`。
EOF

linear issue create \
  --team "ZEE" \
  --project "symphony-test-c2a66ab0f2e7" \
  --state "Todo" \
  --title "<中文 issue 标题>" \
  --description-file "$tmp_issue" \
  --no-interactive
```

After creation, capture the identifier from CLI output or query recent issues:

```sh
linear issue mine --team "ZEE" --project "symphony-test-c2a66ab0f2e7" --all-states --limit 10
linear issue view <ISSUE> --json
```

If the intended flow requires human-controlled start, create the issue in
`Backlog` and let the user move it to `Todo`. If the goal is immediate
processing, create it in `Todo`.

## Start The Listener Fast

First check whether a listener is already running. Do not start a duplicate
service just because an old PID file exists:

```sh
pgrep -fl 'bin/symphony-go run --workflow ./WORKFLOW.md' || true
test -f .symphony/pids/symphony-go.pid && cat .symphony/pids/symphony-go.pid || true
```

For the default issue-run lifecycle, start one issue-scoped listener from the
repo root and stop it after the target issue reaches `Done` or another terminal
state. Do not stop it merely because the issue is in `Human Review`; the user may
move it to `Merging`, and the same listener should continue processing that
state transition.

```sh
make build
mkdir -p .symphony/logs .symphony/pids
ts=$(date +%Y%m%d-%H%M%S)
log=".symphony/logs/${ISSUE:-issue}-$ts.out"
python3 - "$ISSUE" "$log" <<'PY'
import os
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

This avoids tying the listener to the current terminal session. In the latest
smoke run, a plain `nohup ... &` launch exited immediately with an empty daemon
log, while the detached Python launcher stayed alive.

If the user explicitly wants a long-lived listener for all active issues, start
one continuous listener instead:

```sh
WORKFLOW=./WORKFLOW.md MERGE_TARGET=main make run
```

For a non-TUI foreground listener:

```sh
make build
./bin/symphony-go run --workflow ./WORKFLOW.md --no-tui --merge-target main
```

For debugging a single issue once:

```sh
ISSUE=<ISSUE> WORKFLOW=./WORKFLOW.md MERGE_TARGET=main make run-once
```

`run-once` is not the normal listener mode. Use it only to test one poll or
diagnose a specific issue.

## Merging Supervision

Current `WORKFLOW.md` does not use PR land. In `Merging`, the listener should
follow the injected Workflow prompt: merge the local issue branch into local
`main`, validate, push `origin/main`, and move the issue to `Done`.

When the user moves an issue to `Merging`, watch the latest human log:

```sh
latest_log=$(ls -t .symphony/logs/run-*.human.log | head -1)
tail -n 160 "$latest_log"
```

Expected evidence:

- The issue worktree branch has a local task commit.
- Root checkout `main` has a merge commit that includes the issue branch.
- `git push origin main` succeeds.
- Root checkout is aligned: `git status --short --branch` prints
  `## main...origin/main` with no changed files.
- `git worktree list --porcelain` no longer lists `.worktrees/<ISSUE>`.

Do not create a PR in `Merging`. This workflow is optimized for local personal
testing and fast iteration.

Do not pull the remote issue branch. Issue branches are local worktree branches
by default, so `git pull origin <issue-branch>` is expected to fail when the
branch was never pushed. Also do not pre-pull remote `main` on the normal path;
merge the local issue branch into the current local `main`, validate, then push.
Only handle remote synchronization if `git push origin main` fails because the
remote advanced.

If the first Merging attempt fails with `bufio.Scanner: token too long`, do not
restart blindly. Confirm whether the listener retries and whether a second run
continues. If it repeats, treat it as a runner/log-scanner bug and fix that
separately from the issue work.

If `git merge` reports `warning: unable to unlink '<path>': Operation not
permitted`, verify whether the merge commit deleted that exact path but the file
remains as untracked in the root checkout. It is safe to remove that residual
untracked file only when all of these are true:

- `git status --short` shows the same path as `??`.
- `git show --name-status --oneline HEAD` shows that path as deleted.
- The path is explicitly in the issue scope.

After Merging, verify the local repo is aligned and no issue worktree remains:

```sh
git status --short --branch
git rev-parse --short HEAD
git rev-parse --short origin/main
git worktree list --porcelain | rg -n "<ISSUE>|\\.worktrees/<ISSUE>" || true
```

If a background listener is running, keep it alive for future issues. If it was
started only for this issue, stop that issue-scoped listener after the issue is
`Done` or otherwise terminal:

```sh
pid_file=.symphony/pids/<ISSUE>.pid
if [ -f "$pid_file" ]; then
  kill "$(cat "$pid_file")"
  rm "$pid_file"
fi
```

## Background Listener

When the user explicitly wants a background listener for all active issues,
build first and write the PID under `.symphony/pids`:

```sh
make build
mkdir -p .symphony/logs .symphony/pids
ts=$(date +%Y%m%d-%H%M%S)
log=".symphony/logs/daemon-$ts.out"
python3 - "$log" <<'PY'
import os
import subprocess
import sys

log = sys.argv[1]
f = open(log, "ab", buffering=0)
p = subprocess.Popen(
    ["./bin/symphony-go", "run", "--workflow", "./WORKFLOW.md", "--no-tui", "--merge-target", "main"],
    stdin=subprocess.DEVNULL,
    stdout=f,
    stderr=subprocess.STDOUT,
    start_new_session=True,
)
with open(".symphony/pids/symphony-go.pid", "w") as out:
    out.write(str(p.pid))
print(p.pid)
PY
echo "pid=$(cat .symphony/pids/symphony-go.pid) log=$log"
```

Verify it is alive:

```sh
pid=$(cat .symphony/pids/symphony-go.pid)
ps -p "$pid" -o pid,ppid,etime,command
tail -n 80 "$log"
```

If the process exits immediately, do not keep restarting blindly. Read the
daemon log and the latest `.symphony/logs/run-*.human.log` first.

## Verify Processing Started

Check these signals before reporting success:

```sh
linear issue view <ISSUE> --json
ls -la .worktrees
ls -t .symphony/logs/run-*.human.log | head -1
tail -n 120 "$(ls -t .symphony/logs/run-*.human.log | head -1)"
```

Expected signs:

- issue moves from `Todo` to `In Progress`, or logs explain why it is skipped.
- `.worktrees/<ISSUE>` exists after dispatch.
- human log contains `state_changed`, `workpad_updated`, or `codex_session_started`
  for the issue.

## Stop And Review The Run

For the default issue-scoped lifecycle, stop the listener after the target issue
is terminal, then perform a short review before reporting back:

```sh
pid_file=.symphony/pids/<ISSUE>.pid
if [ -f "$pid_file" ]; then
  kill "$(cat "$pid_file")" 2>/dev/null || true
  rm "$pid_file"
fi
pgrep -fl 'symphony-go run --workflow ./WORKFLOW.md|bin/symphony-go run' || true
```

Review and improve from the run:

- Skill gaps: wrong team key, stale PR/land wording, fragile launch command, or
  missing terminal verification.
- Workflow gaps: instructions that caused unnecessary remote pulls, PR work, or
  ambiguity between repo root and issue worktree.
- Code gaps: behavior that should be handled by the orchestrator rather than by
  human cleanup. Check these code anchors when relevant:
  - `cmd/symphony-go/main.go`: run flags and listener lifecycle. A useful future
    improvement is an issue-scoped mode that exits automatically once the target
    issue reaches a terminal state.
  - `internal/orchestrator/orchestrator.go`: `Merging` workflow prompt injection,
    terminal cleanup, and workpad wording. Watch for delayed worktree cleanup
    after terminal state and stale standalone-skill labels.
  - `internal/workspace/workspace.go` and `scripts/symphony_before_remove.sh`:
    worktree cleanup behavior and root checkout residue after merge.

Only make code changes when the root cause is clear and the change is small
enough to verify immediately. Otherwise record a follow-up issue with concrete
file anchors and reproduction evidence.

## Common Mistakes

- Do not create the issue in a state outside `WORKFLOW.md` active states unless
  the user explicitly wants to hold it.
- Do not run only `make run-once` when the user asked for a listener service.
- Do not start a second listener when the first one is already polling; duplicate
  listeners can dispatch the same issue twice.
- Do not leave an issue-scoped listener running after the target issue reaches
  `Done` or another terminal state.
- Do not use Linear MCP/app tools from unattended child agents.
- Do not hardcode another repository path, remote, branch, or project.
- Do not declare the listener healthy just because a PID file exists; confirm
  with `ps` and logs.
- Do not delete `.worktrees/<ISSUE>` manually while the issue is active.
- Do not declare Merging complete until root checkout, `origin/main`, Linear
  state, and worktree cleanup have all been checked.
- After the issue is complete and the listener is stopped, review the run logs
  and update this skill or `WORKFLOW.md` with any concrete failure mode that
  caused delay or manual recovery.
- Include code-level optimization notes in the handoff when the run exposed
  orchestrator behavior that should become first-class code.

## Handoff

Report back with:

- Linear issue identifier and URL.
- Listener mode: foreground, background, or `run-once`.
- PID and log path if background mode was used.
- Current issue state.
- Worktree path if created.
- Merge commit and push evidence if Merging ran.
- Root checkout status after merge.
- Latest relevant log evidence.
- Whether the issue-scoped listener was stopped.
