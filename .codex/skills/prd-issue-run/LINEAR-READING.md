# Linear Reading Protocol

Use this protocol before selecting a child issue or closing a parent PRD.

## Pitfalls

- Do not rely on `linear issue query` table output alone. It can truncate labels, for example `ready-for-human` appears as `ready-for...`.
- If the CLI reports no default team, add `--all-teams`.
- Use `linear issue comment list <ISSUE>`, not `linear issue comments <ISSUE>`.
- `linear issue relation list <ISSUE>` may not show every rendered child. `linear issue view <ISSUE>` can still include a `Sub-issues` section.

## Read Pattern

```bash
linear issue query --all-teams --state backlog --limit 100 -j
linear issue query --all-teams --state unstarted --limit 100 -j
linear issue query --all-teams --state triage --limit 100 -j
linear issue query --all-teams --label ready-for-agent --limit 100 -j
linear issue query --all-teams --label needs-triage --limit 100 -j
linear issue view <ISSUE>
linear issue comment list <ISSUE>
linear issue relation list <ISSUE>
```

Use JSON query output for exact state, labels, assignee, project, and team.
Use `linear issue view` for description, Sub-issues, attachments, checklist, and rendered parent/child context.

After every Linear update, re-read with:

```bash
linear issue view <ISSUE>
```

Verify the exact state, checklist, comments, and attachments before claiming completion.
