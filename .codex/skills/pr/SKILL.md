---
name: pr
description: Use when Symphony Go reaches Merging for a GitHub PR based merge flow.
---

# PR

This skill is scoped to `/Users/bytedance/symphony-go`.

## Goals

- Turn the current issue worktree branch into a GitHub PR if one does not exist.
- Keep the branch, PR title, PR body, and visible comments up to date in Chinese.
- Wait for checks and review feedback, then squash-merge the PR.
- Pull the merged result back into the root `main` checkout.
- Remove the issue worktree only after the merge is visible on `origin/main`.

Do not delegate to another repo skill. This file is the complete PR merge flow.

## Preconditions

- Run from the issue worktree, not from `main`.
- `gh` is authenticated for the repo.
- The merge target is `main` unless the orchestrator prompt says otherwise.

## Steps

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
8. Sync root checkout after merge:
   - `git -C "$repo_root" fetch origin main`.
   - If the root checkout has local runtime edits, only clear paths that are
     byte-for-byte identical to `origin/main`. Stop on any divergent local edit.
   - `git -C "$repo_root" switch main`
   - `git -C "$repo_root" pull --ff-only origin main`
9. Remove the issue worktree:
   - `git worktree remove "$issue_worktree"`.
   - If removal refuses because of local changes, stop and report
     `git -C "$issue_worktree" status --short --branch` and
     `git -C "$issue_worktree" diff --stat`.
10. Report evidence in the workpad: PR URL, merge commit/result, root pull
    result, and worktree cleanup result.

## Root Checkout Safety

The root checkout may contain temporary runtime copies of the same changes that
were just merged. Before pulling, clear only files that exactly match
`origin/main`; this avoids deleting unrelated user edits.

```sh
repo_root=/Users/bytedance/symphony-go
git -C "$repo_root" fetch origin main

dirty=$(git -C "$repo_root" status --porcelain)
if [ -n "$dirty" ]; then
  printf '%s\n' "$dirty" | while IFS= read -r line; do
    path=${line#???}
    case "$line" in
      '?? '*)
        if git -C "$repo_root" show "origin/main:$path" >/tmp/pr-origin-file 2>/dev/null \
          && cmp -s "$repo_root/$path" /tmp/pr-origin-file; then
          rm -f "$repo_root/$path"
        else
          echo "root checkout has divergent untracked file: $path" >&2
          exit 1
        fi
        ;;
      *)
        if git -C "$repo_root" diff --quiet origin/main -- "$path"; then
          git -C "$repo_root" restore --staged --worktree -- "$path"
        else
          echo "root checkout has divergent local edit: $path" >&2
          exit 1
        fi
        ;;
    esac
  done
fi

git -C "$repo_root" switch main
git -C "$repo_root" pull --ff-only origin main
```

## Failure Handling

- If auth, permission, missing secret, or branch protection blocks the flow,
  stop and report the exact command and error.
- If mergeability is `UNKNOWN`, wait and re-check.
- If mergeability is `CONFLICTING`, fetch `origin/main`, resolve conflicts in
  the issue worktree, rerun validation, commit, and push.
- Do not force-remove worktrees or reset the root checkout unless every affected
  path has been proven identical to `origin/main`.
