---
name: tdd-acceptance-pr
description: Review a TDD-completed Linear issue against acceptance criteria, tick proven checklist items, and run the Symphony Go PR merge flow. Use when TDD is green and the user asks to review acceptance criteria, check boxes, create/push/merge a PR, or verify Linear auto-Done.
---

# TDD Acceptance PR

This skill is scoped to `/Users/bytedance/symphony-go`.

Use after TDD is green and the next job is acceptance review plus PR merge. This
skill is high-cohesion: it owns checklist review, Linear workpad evidence, PR
creation, merge, root sync, and Linear Done verification. It may call the
deterministic PR script, but do not reduce this flow to "go use another skill".

## Preconditions

- Work from the issue branch/worktree. If changes are on root `main`, move them
  to the requested feature branch/worktree without losing files.
- Identify the Linear issue id from prompt, branch, PR body, or worktree path.
- `gh` must be authenticated. In the current MCP smoke workflow, write Linear
  through Linear MCP/app tools and do not fall back to `linear_graphql` or
  `linear` CLI.
- If the TDD evidence is stale or interrupted, rerun the smallest relevant tests.

## Acceptance Review

1. Read the issue description, comments, and existing `## Codex Workpad`.
2. Extract checkboxes from the issue description first. If they only exist in a
   triage/comment, copy them into the workpad review section.
3. For each box, write evidence: file path, command, diff, or out-of-scope note.
4. Only change `[ ]` to `[x]` when current code and validation prove it.
5. If any in-scope criterion cannot be proven, stop before PR merge and write the
   blocker, exact command/output, and next action to the workpad.
6. If the user says to ignore unchecked boxes, record that instruction and continue, but do not claim ignored boxes are done.

## Local Verification

```bash
git status --short --branch
git diff --check
```

Then run the smallest relevant test/build set. Prefer repo wrappers:

```bash
./test.sh ./internal/<package>
make test
make build
```

If a full test has an unrelated flaky failure, rerun the exact failing test once and then rerun the full command. Continue only when final evidence is green.

## Workpad Update

Maintain one persistent `## Codex Workpad` comment in Chinese. Update it with:

- current branch and issue id;
- acceptance checklist with `[x]` only for proven items;
- validation commands and pass/fail result;
- PR URL, merge commit, and root `main` sync result after merge;
- Linear auto-Done observation after merge.

Do not create extra summary comments unless the workpad cannot be updated.

## PR Merge Flow

Prepare a Chinese PR title/body with `Linear: <ISSUE>`, then run:

```bash
repo_root=/Users/bytedance/symphony-go
tmp_body=$(mktemp)
cat > "$tmp_body" <<'EOF'
## 摘要
- <中文概括本次变更>
## 验证
- `git diff --check`
- `<相关 ./test.sh / make test / make build 命令>`
Linear: <ISSUE>
EOF

"$repo_root/.codex/skills/pr/scripts/pr_merge_flow.sh" \
  --repo-root "$repo_root" \
  --target main --commit-message "<type(scope): 中文提交信息>" \
  --pr-title "<中文 PR 标题>" \
  --pr-body-file "$tmp_body"
```

The script commits, pushes, creates/updates the PR, waits for checks,
squash-merges, and fast-forwards root `main`. If it stops after side effects,
inspect live PR/branch/root/Linear state before retrying.

## Done Verification

After the PR script succeeds:

1. Confirm `gh pr view <PR> --json state,mergedAt,mergeCommit,url`.
2. Confirm root `main`: `git status --short --branch` and `git log -1 --oneline --decorate`.
3. Re-read the Linear issue. If it auto-moved to `Done`, record that evidence in the workpad.
4. If Linear did not auto-move to `Done`, do not silently mark it done. Record the missing automation evidence and ask whether to manually transition it.
