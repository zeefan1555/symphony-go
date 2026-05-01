---
name: linear-cli
description: Use when unattended Symphony/Codex runs need to read or write Linear through the `linear` CLI, especially when Linear MCP/app tools would trigger interactive approval.
---

# Linear CLI

## Scope

This skill is scoped to `/Users/bytedance/symphony-go` and the `linear` CLI
from `schpet/linear-cli`.

Use this skill for unattended Linear operations when `linear_graphql` is not
available or when a shell command is simpler. Do not call Linear MCP/app tools
from a child Codex session; they can trigger interactive approval and fail the
run.

## Preconditions

- `linear --version` works.
- `LINEAR_API_KEY` is set, or `linear auth whoami` succeeds through stored
  credentials.
- Commands run from the issue worktree unless a workflow explicitly says
  otherwise.

## Quick Checks

```bash
linear --version
linear auth whoami
linear issue view ZEE-37 --json
linear issue comment list ZEE-37
```

## Common Operations

Read an issue with comments:

```bash
linear issue view <ISSUE> --json
```

Update state:

```bash
linear issue update <ISSUE> --state "In Progress"
linear issue update <ISSUE> --state "Human Review"
```

Add a short comment:

```bash
linear issue comment add <ISSUE> --body "中文进度说明"
```

Update an existing `## Codex Workpad` comment:

```bash
linear issue comment list <ISSUE>
linear issue comment update <COMMENT_ID> --body-file /tmp/workpad.md
```

Link a GitHub PR:

```bash
linear issue link <ISSUE> https://github.com/zeefan1555/symphony-go/pull/<PR>
```

## Rules

- Prefer one persistent `## Codex Workpad` comment; update it in place.
- Write Linear comments in Chinese for this workflow.
- Use `--body-file` for Markdown longer than one sentence.
- Keep command output in workpad notes short: command, result, and blocker.
- If the CLI reports auth or permission errors, record the exact command and
  error as a blocker instead of falling back to MCP/app tools.
