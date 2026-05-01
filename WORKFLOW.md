---
tracker:
  kind: linear
  api_key: lin_api_anKemYvMitkjBS2cPmSQHtaAW40DEuvZBeI6EuJn
  project_slug: "symphony-test-c2a66ab0f2e7"
  active_states:
    - Todo
    - In Progress
    - Human Review
    - Merging
    - Rework
  terminal_states:
    - Closed
    - Cancelled
    - Canceled
    - Duplicate
    - Done
polling:
  interval_ms: 5000
workspace:
  root: .worktrees
hooks:
  after_create: |
    workspace="$(pwd -P)"
    go_root="$(cd "$workspace/../.." && pwd -P)"
    "$go_root/scripts/symphony_after_create.sh" "$workspace"
  before_remove: |
    workspace="$(pwd -P)"
    go_root="$(cd "$workspace/../.." && pwd -P)"
    "$go_root/scripts/symphony_before_remove.sh" "$workspace"
agent:
  max_concurrent_agents: 10
  max_turns: 20
codex:
  command: codex --config shell_environment_policy.inherit=all --config 'model="gpt-5.5"' --config model_reasoning_effort=xhigh app-server
  approval_policy: never
  thread_sandbox: workspace-write
  turn_sandbox_policy:
    type: workspaceWrite
    writableRoots:
      - /Users/bytedance/symphony/go/.worktrees
      - /Users/bytedance/symphony/.git
    readOnlyAccess:
      type: fullAccess
    networkAccess: true
    excludeTmpdirEnvVar: false
    excludeSlashTmp: false
---

You are working on a Linear ticket `{{ issue.identifier }}`.

{% if attempt %}
Continuation context:

- This is retry attempt #{{ attempt }} because the ticket is still active.
- Resume from the current worktree. Do not redo completed changes.
- If a local commit already exists, check whether only validation or review handoff remains.
{% endif %}

Issue context:
Identifier: {{ issue.identifier }}
Title: {{ issue.title }}
Current status: {{ issue.state }}
Labels: {{ issue.labels }}
URL: {{ issue.url }}

Description:
{% if issue.description %}
{{ issue.description }}
{% else %}
No description provided.
{% endif %}

## Goal

This is a local smoke workflow. Do not create a PR and do not push.

The agent should:

1. Make the smallest required change in the issue worktree.
2. Run local validation that directly proves the change.
3. Create one local git commit.

The Go orchestrator, not the Codex agent, will update the single `## Codex Workpad` Linear comment and move the issue to `Human Review` after it detects a local commit.

After manual approval, the human will move the issue to `Merging`. In `Merging`, follow `.codex/skills/local-merge/SKILL.md` and merge the issue branch into the outer repository branch `feat_zff`, validate, then move the issue to `Done`.

## Rules

- Keep all Linear workpad notes, status notes, final handoff text, and agent-facing progress text in Chinese when the issue is a Chinese smoke test.
- Work only inside the provided repository and worktree.
- Prefer the smallest useful change. Do not refactor, format unrelated files, or expand scope.
- If the issue description includes validation commands, run them. Otherwise run the smallest relevant checks.
- Do not use the GitHub PR flow. Do not run `git push` or `gh pr create`.
- Do not stop early unless blocked by missing required auth, permissions, or tools.
- Do not call Linear MCP, Linear apps, or any MCP write tool. Interactive MCP approval is not available in unattended runs.
- Do not update Linear comments or issue status yourself. The Go orchestrator writes the Workpad comment in Chinese and moves state through Linear GraphQL after your local commit is present.

## State Routing

- `Backlog`: do not modify the issue.
- `Todo`: move to `In Progress`, then implement locally.
- `In Progress`: implement, validate, and commit locally. The Go orchestrator updates the workpad and moves the issue to `Human Review`.
- `Human Review`: do not edit files. Wait for manual review.
- `Rework`: address review feedback in the same worktree, validate, create a new local commit, then move back to `Human Review`.
- `Merging`: use the `local-merge` skill to merge the issue branch into outer repository branch `feat_zff`, validate, update workpad, then move to `Done`.
- `Done`: terminal state. Do nothing.

## In Progress Flow

1. Fetch the issue and read its current state.
2. If the issue is `Todo`, move it to `In Progress`.
3. Read the existing `## Codex Workpad` context if present, but do not edit Linear comments.
4. Use this environment stamp for local reasoning only:

   ```text
   <hostname>:<abs-worktree-path>@<short-sha>
   ```

5. Reproduce or confirm the target signal before editing. For documentation smoke tasks, use `rg` to show the target text is currently absent.
6. Make the smallest required change.
7. Run validation:
   - Run all validation commands from the issue description when present.
   - Always run `git diff --check`.
   - For documentation smoke tasks, run an `rg` command proving the target text is present.
8. Check the change scope:

   ```bash
   git diff --name-only
   git status --short
   ```

9. Create a local commit. Include the issue identifier in the commit message, for example:

   ```text
   ZEE-8: update README smoke marker
   ```

10. Stop after the local commit exists. The Go orchestrator will:
    - Update the Workpad comment.
    - Record changed files and the commit hash.
    - Record orchestration validation status.
    - Move the issue to `Human Review`.
11. Final message must report completed actions and blockers only.

## Human Review Flow

- `Human Review` means a local commit is ready for manual review.
- Do not edit files in this state.
- If changes are requested, the human will move the issue to `Rework`.
- If approved, the human will move the issue to `Merging`.

## Rework Flow

1. Read the issue comments and review request.
2. Make the smallest required change in the same worktree.
3. Re-run relevant validation.
4. Create a new local commit.
5. Update the same workpad with the new commit and validation result.
6. Move the issue back to `Human Review`.

## Merging Flow

1. Open and follow `.codex/skills/local-merge/SKILL.md`.
2. Confirm the current worktree branch is the issue branch, such as `symphony/{{ issue.identifier }}`.
3. Confirm the issue worktree is clean:

   ```bash
   git status --short
   ```

4. Locate the outer repository checkout and confirm branch `feat_zff` exists.
5. Switch the outer repository checkout to `feat_zff` only when its working tree is clean or contains only changes you created for this task.
6. Merge the issue branch into local `feat_zff`:

   ```bash
   git checkout feat_zff
   git merge --no-ff <issue-branch>
   ```

7. Do not create a temporary clone or temporary main worktree as a fallback. If the real outer checkout cannot be merged safely, update the workpad with a blocker and leave the issue in `Merging`.
8. Run validation relevant to the change. At minimum run `git diff --check HEAD~1..HEAD` or an equivalent check.
9. Update the workpad with the merge commit, validation result, and local `feat_zff` HEAD.
10. Move the issue to `Done`.

## Workpad Template

Use one persistent comment and update it in place:

````md
## Codex Workpad

```text
<hostname>:<abs-worktree-path>@<short-sha>
```

### Plan

- [ ] Confirm state and target
- [ ] Make the smallest required change
- [ ] Run local validation
- [ ] Create a local commit
- [ ] Wait for manual review

### Acceptance Criteria

- [ ] Change satisfies the issue description
- [ ] Change scope is expected
- [ ] Local commit exists

### Validation

- [ ] `git diff --check`

### Notes

- <timestamp>: <progress, changed files, commit, validation result, or blocker>
````
