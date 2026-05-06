---
name: linear-cli
description: Legacy fallback for reading or writing Linear through the `linear` CLI. Do not use for the current MCP smoke workflow unless the user explicitly asks to test CLI fallback.
---

# Linear CLI

## Scope

This skill is scoped to `/Users/bytedance/symphony-go` and the `linear` CLI
from `schpet/linear-cli`.

Use this skill only for legacy CLI fallback checks. The current Symphony Go MCP
smoke workflow intentionally tests Linear MCP/app availability in child Codex
sessions, so child agents must not read this skill or fall back to the CLI unless
the user explicitly changes the smoke goal.

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
