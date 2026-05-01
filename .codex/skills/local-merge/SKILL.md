---
name: local-merge
description:
  Merge a local Symphony issue worktree branch into the local main checkout
  during the Linear Merging state; use for direct workflows that do not create
  PRs.
---

# Local Merge

## Goals

- Merge the reviewed issue branch into the real local `main` checkout.
- Push the updated `main` branch after merge validation when the workflow uses
  the direct local merge path.
- Keep the flow small, deterministic, and easy to audit.
- Fail fast when the real local checkout cannot be merged safely.
- Avoid slow fallback paths that make a smoke workflow look successful without
  changing the intended checkout.

## Use When

- The Linear issue is in `Merging`.
- The workflow is direct: no PR and no GitHub merge button.
- The issue branch already has a reviewed local commit.
- The desired final state is `Done` after local merge validation and direct
  push pass.

## Hard Rules

- Do not create a temporary clone to simulate the merge.
- Do not create a temporary main worktree to bypass a dirty or locked checkout.
- Do not switch or modify a dirty checkout that contains unowned changes.
- Do not move the Linear issue to `Done` unless the real local `main` checkout
  contains the merge commit and the intended remote push has succeeded.
- If local `main` has extra ahead commits beyond the issue merge, push them only
  when they are explicitly part of the current workflow session and documented
  in the workpad; otherwise stop and record a blocker.
- If Git cannot write `.git/worktrees/<issue>/index.lock`, treat it as a
  sandbox/runtime problem. Record the blocker instead of retrying repeatedly.
- If the repository is running under Symphony/Codex sandboxing, confirm the
  service process was restarted after any sandbox-root code changes.

## Preconditions

- You are in the issue worktree, for example:

  ```bash
  /Users/bytedance/symphony-go/.worktrees/ZEE-8
  ```

- The issue worktree is clean:

  ```bash
  git status --short
  ```

- The issue branch is the expected branch:

  ```bash
  git branch --show-current
  ```

- The real local main checkout exists and is clean.

## Steps

1. Identify the issue branch and issue worktree.

   ```bash
   issue_branch=$(git branch --show-current)
   issue_head=$(git rev-parse --short HEAD)
   git status --short
   ```

2. Locate the real local main checkout from `git worktree list`.

   ```bash
   git worktree list --porcelain
   ```

   Pick the worktree whose branch is `refs/heads/main`.

   If no real local `main` checkout exists, stop and record a blocker.

3. Check the real main checkout is safe to touch.

   ```bash
   git -C "$main_checkout" branch --show-current
   git -C "$main_checkout" status --short
   ```

   If it is not on `main`, or if status is not clean, stop and record a blocker.
   Do not stash, reset, checkout over, or clean unowned changes.

4. Merge the issue branch into real local main.

   ```bash
   git -C "$main_checkout" merge --no-ff "$issue_branch" \
     -m "Merge $issue_branch into main"
   ```

   If this fails with `index.lock` or `Operation not permitted`, stop and record
   a sandbox/runtime blocker. Do not retry through clone-based fallback.

5. Validate the merge.

   ```bash
   git -C "$main_checkout" diff --check HEAD~1..HEAD
   git -C "$main_checkout" diff --name-only HEAD~1..HEAD
   git -C "$main_checkout" status --short
   git -C "$main_checkout" log --oneline --decorate -3
   ```

   Add any issue-specific validation from the workpad or issue description.

6. Push the target branch directly.

   ```bash
   git -C "$main_checkout" push origin main
   ```

   If push fails because `origin/main` advanced, stop and record a blocker.
   Do not force-push `main`.
   If `main` is already ahead because the reviewed issue merge and documented
   workflow follow-up commits are present locally, push that exact ahead range
   after validation instead of trying to rewrite history.

7. Update the single Linear workpad comment.

   Record:

   - issue branch
   - issue commit
   - local main checkout path
   - merge commit
   - validation commands and results
   - ahead commits included in the push
   - push command and result
   - any blocker if merge did not complete

8. Move the issue to `Done` only after the real local main checkout contains the
   merge commit, validation passed, and direct push succeeded.

## Blocker Template

Use this in the workpad when merge cannot proceed:

```md
### Merge Blocker

- Real main checkout: `<path or missing>`
- Issue branch: `<branch>`
- Issue commit: `<sha>`
- Blocker: `<dirty main checkout | missing main checkout | sandbox index.lock | merge conflict | validation failure>`
- Evidence: `<command and key output>`
- Impact: local `main` was not updated, so the issue was not moved to `Done`.
- Unblock action: run the merge from a clean real `main` checkout with write access to the repository `.git`, then rerun validation and push.
```

## Notes For Symphony Local Smoke

- The sandbox-root fix for git worktree metadata lives in
  the Codex app-server sandbox policy in this repository. A running Symphony
  service must be restarted to use that code.
- A successful merge in a throwaway clone is only a proof that the branch is
  mergeable. It is not completion of the local workflow.
- Keep this skill short in the workflow prompt. The agent should not re-read the
  full generic PR `land` skill for local smoke merges.
