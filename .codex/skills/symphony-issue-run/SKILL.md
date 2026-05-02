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
  the Linear project, active states, workspace root, review policy, and
  state-to-skill routing.

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
- Linear 评论、GitHub PR 标题和正文默认使用中文。
- 如果人工把 issue 切到 `Merging`，由 `agent.state_skills.Merging`
  指向的 repo-root skill 负责 land 已存在 PR 和 root 回拉；PR 必须已在
  `Human Review` 前创建，worktree 清理由 orchestrator workspace manager 完成。
EOF

linear issue create \
  --team "Zeefan" \
  --project "symphony-test-c2a66ab0f2e7" \
  --state "Todo" \
  --title "<中文 issue 标题>" \
  --description-file "$tmp_issue" \
  --no-interactive
```

After creation, capture the identifier from CLI output or query recent issues:

```sh
linear issue list --team "Zeefan" --project "symphony-test-c2a66ab0f2e7" --limit 10
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

For normal operation, start one continuous listener from the repo root:

```sh
WORKFLOW=./WORKFLOW.md MERGE_TARGET=main make run
```

This builds `bin/symphony-go` and starts polling all active issues. It is the
preferred path when the user wants the service to keep handling future issues.

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

Merging operating details belong to `.codex/skills/land/SKILL.md`, not here.
This skill only supervises whether the Merging state completed end to end.

When the user moves an issue to `Merging`, watch the latest human log:

```sh
latest_log=$(ls -t .symphony/logs/run-*.human.log | head -1)
tail -n 160 "$latest_log"
```

Expected evidence:

- `merge_skill_started` for the issue and `.codex/skills/land/SKILL.md`.
- An existing GitHub PR URL, usually attached back to the Linear issue before
  `Human Review`.
- PR state becomes `MERGED`.
- Root checkout is aligned: `git status --short --branch` prints
  `## main...origin/main` with no changed files.
- `git worktree list --porcelain` no longer lists `.worktrees/<ISSUE>`.

If no PR exists when the issue enters `Merging`, treat it as a workflow
violation. Do not create a new PR inside `Merging`; record the blocker in the
workpad and route the issue back to implementation/rework.

If the first Merging attempt fails with `bufio.Scanner: token too long`, do not
restart blindly. Confirm whether the listener retries and whether a second run
continues. If it repeats, treat it as a runner/log-scanner bug and fix that
separately from the issue work.

If PR merge succeeded and the worktree was removed but root is still behind,
perform safe recovery only after proving root runtime copies are identical to
`origin/main`; stop on any divergent local edit.

After Merging, verify the local repo is aligned and no issue worktree remains:

```sh
git status --short --branch
git rev-parse --short HEAD
git rev-parse --short origin/main
git worktree list --porcelain | rg -n "<ISSUE>|\\.worktrees/<ISSUE>" || true
```

If a background listener is running, keep it alive for future issues. If it was
started only for this issue, stop that issue-scoped listener after cleanup:

```sh
pid_file=.symphony/pids/<ISSUE>.pid
if [ -f "$pid_file" ]; then
  kill "$(cat "$pid_file")"
  rm "$pid_file"
fi
```

## Background Listener

When the user explicitly wants a background listener, build first and write the
PID under `.symphony/pids`:

```sh
make build
mkdir -p .symphony/logs .symphony/pids
ts=$(date +%Y%m%d-%H%M%S)
log=".symphony/logs/daemon-$ts.out"
nohup ./bin/symphony-go run --workflow ./WORKFLOW.md --no-tui --merge-target main > "$log" 2>&1 &
echo $! > .symphony/pids/symphony-go.pid
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

## Common Mistakes

- Do not create the issue in a state outside `WORKFLOW.md` active states unless
  the user explicitly wants to hold it.
- Do not run only `make run-once` when the user asked for a listener service.
- Do not start a second listener when the first one is already polling; duplicate
  listeners can dispatch the same issue twice.
- Do not use Linear MCP/app tools from unattended child agents.
- Do not hardcode another repository path, remote, branch, or project.
- Do not declare the listener healthy just because a PID file exists; confirm
  with `ps` and logs.
- Do not delete `.worktrees/<ISSUE>` manually while the issue is active.
- Do not declare Merging complete until PR state, root checkout, Linear state,
  and worktree cleanup have all been checked.

## Handoff

Report back with:

- Linear issue identifier and URL.
- Listener mode: foreground, background, or `run-once`.
- PID and log path if background mode was used.
- Current issue state.
- Worktree path if created.
- PR URL and merge commit if Merging ran.
- Root checkout status after merge.
- Latest relevant log evidence.
