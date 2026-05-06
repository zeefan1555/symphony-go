---
name: pr
description: Use when Symphony Go reaches Merging for a GitHub PR based merge flow.
---

# PR

This skill is scoped to `/Users/bytedance/symphony-go`.

Use the bundled script for the mechanical PR flow. The agent's job is to
prepare the PR title/body and inspect blockers; the script owns push, PR
creation/update, checks, and squash merge. Repo-root `main` checkout sync is not
owned by the issue worktree agent because the agent may not have stable write
permission outside the issue worktree. The Go orchestrator/operator owns root
checkout sync and issue worktree cleanup after this skill returns successfully.

## Goals

- Turn the current issue worktree branch into a GitHub PR if one does not exist.
- Keep the branch, PR title, PR body, and visible comments up to date in Chinese.
- Wait for checks and review feedback, then squash-merge the PR.
- Fetch `origin/main` and report root checkout status, but do not pull or mutate
  the repo-root `main` checkout from the issue worktree agent.
- Leave issue worktree cleanup to the Go orchestrator after the merge is visible
  on `origin/main`.

Do not delegate to another repo skill. This file is the complete PR merge flow.

## Preconditions

- Run from the issue worktree, not from `main`.
- `gh` is authenticated for the repo.
- The merge target is `main` unless the orchestrator prompt says otherwise.

## Primary Command

Run from the issue worktree, after reading the current diff and choosing a
Chinese PR title/body:

```sh
repo_root=/Users/bytedance/symphony-go
tmp_body=$(mktemp)
cat > "$tmp_body" <<'EOF'
## 摘要

- <用中文概括本次变更>

## 验证

- `git diff --check`
- `make build`

Linear: <ISSUE>
EOF

"$repo_root/.codex/skills/pr/scripts/pr_merge_flow.sh" \
  --repo-root "$repo_root" \
  --target main \
  --commit-message "<type(scope): 中文提交信息>" \
  --pr-title "<中文 PR 标题>" \
  --pr-body-file "$tmp_body"
```

The script prints the PR URL, merge commit, root checkout status, and root sync
ownership note. Copy those facts into the persistent Linear workpad in Chinese.
After the script finishes, do not remove the issue worktree manually and do not
pull the repo-root checkout from the issue worktree agent; return success so the
orchestrator/operator can run the configured root sync and workspace cleanup
paths.

## Script Flow

1. Identify context:
   - `branch=$(git branch --show-current)`
   - stop if `branch` is empty, `main`, or the merge target branch.
   - infer `issue_id` from the branch name or worktree directory.
2. Commit any intended local changes in the issue worktree:
   - run `git status --short --branch`;
   - if there are uncommitted changes, run `git diff --check`, `make build`,
     then commit with a Chinese-aware but conventional subject such as
     `chore(workflow): 切换 PR merge flow`.
   - if `make build` fails only because the external Go cache is not writable,
     rerun `GOCACHE=$PWD/.cache/go-build make build`, then remove `.cache`
     before committing.
3. Push the branch:
   - `git push -u origin HEAD`
   - if rejected because the remote moved, fetch and merge/rebase only after
     inspecting the exact error; never rewrite remote history without
     `--force-with-lease`.
4. Create or update the PR:
   - if `gh pr view` fails, run `gh pr create` with a Chinese title/body.
   - if a PR exists, refresh its title/body so it matches the full branch diff.
   - body fallback sections: `## 摘要`, `## 验证`, `Linear`.
   - add the `symphony` label; create the label if it is missing.
5. Check merge readiness:
   - run `gh pr view --json number,title,body,mergeable,state,url`;
   - inspect review comments and top-level PR comments;
   - reply in Chinese with `[codex]` before changing code for review feedback.
6. Wait for checks:
   - use `gh pr checks --watch`;
   - if checks fail, inspect logs, fix in the issue worktree, commit, push, and
     repeat from step 4.
7. Squash-merge:
   - use the PR title/body as merge subject/body;
   - `gh pr merge --squash --subject "$pr_title" --body "$pr_body"`.
8. Fetch `origin/main` and report root checkout status. Do not switch, restore,
   delete, or pull the repo-root checkout from the issue worktree agent.
9. Report evidence in the workpad: PR URL, merge commit/result, root status, and
   the note that root checkout sync is owned by the orchestrator/operator. Do
   not remove the issue worktree in this skill; cleanup is owned by the
   orchestrator workspace manager.

## Manual Fallback

Use the step-by-step commands manually only if
`.codex/skills/pr/scripts/pr_merge_flow.sh` is missing, not executable, or exits
before creating side effects. If the script exits after creating or merging a
PR, inspect the current PR/root/worktree state before retrying.

## Root Checkout Safety

The root checkout may contain unrelated user edits or files protected by the
runner sandbox. The issue worktree agent must not switch, restore, delete, or
pull the repo-root checkout. If root sync is needed for local developer
convenience after PR merge, it must be handled by the orchestrator/operator from
the repo-root context after verifying the root checkout is clean or explicitly
safe to update.

## Failure Handling

- If auth, permission, missing secret, or branch protection blocks the flow,
  stop and report the exact command and error.
- If mergeability is `UNKNOWN`, wait and re-check.
- If mergeability is `CONFLICTING`, fetch `origin/main`, resolve conflicts in
  the issue worktree, rerun validation, commit, and push.
- Do not force-remove worktrees, reset the root checkout, restore root files, or
  pull root `main` from the issue worktree agent.
- If `gh pr checks --watch` says `no checks reported`, treat it as no remote
  checks to wait for; rely on the local validation already run by the script.
