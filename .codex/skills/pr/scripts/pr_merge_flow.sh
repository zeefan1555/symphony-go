#!/usr/bin/env bash
set -euo pipefail

repo_root="/Users/bytedance/symphony-go"
target="main"
commit_message=""
pr_title=""
pr_body_file=""

usage() {
  cat <<'EOF'
Usage: pr_merge_flow.sh [--repo-root PATH] [--target BRANCH] [--commit-message MSG] [--pr-title TITLE] [--pr-body-file FILE]

Run the Symphony PR merge flow from an issue worktree:
  validate, optionally commit local issue changes, push, create/update PR,
  wait for checks, squash merge, and report root checkout status.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --repo-root)
      repo_root="$2"
      shift 2
      ;;
    --target)
      target="$2"
      shift 2
      ;;
    --commit-message)
      commit_message="$2"
      shift 2
      ;;
    --pr-title)
      pr_title="$2"
      shift 2
      ;;
    --pr-body-file)
      pr_body_file="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

repo_root="$(cd "$repo_root" && pwd -P)"
issue_worktree="$(pwd -P)"
branch="$(git branch --show-current)"

if [ -z "$branch" ] || [ "$branch" = "$target" ] || [ "$issue_worktree" = "$repo_root" ]; then
  echo "must run from an issue worktree branch, got branch=$branch cwd=$issue_worktree" >&2
  exit 1
fi

if ! command -v gh >/dev/null 2>&1; then
  echo "missing gh CLI" >&2
  exit 1
fi

if [ -z "$commit_message" ]; then
  commit_message="chore(workflow): 合并 ${branch}"
fi

if [ -z "$pr_title" ]; then
  pr_title="$(git log -1 --pretty=%s 2>/dev/null || true)"
  if [ -z "$pr_title" ]; then
    pr_title="合并 ${branch}"
  fi
fi

tmp_build_log="$(mktemp)"
tmp_body=""
cleanup() {
  rm -f "$tmp_build_log"
  if [ -n "$tmp_body" ]; then
    rm -f "$tmp_body"
  fi
}
trap cleanup EXIT

run_build() {
  if make build >"$tmp_build_log" 2>&1; then
    return 0
  fi
  if grep -q "operation not permitted" "$tmp_build_log" && grep -q "go-build" "$tmp_build_log"; then
    GOCACHE="$issue_worktree/.cache/go-build" make build
    if [ -d "$issue_worktree/.cache" ]; then
      find "$issue_worktree/.cache" -type f -delete
      find "$issue_worktree/.cache" -type d -empty -delete
    fi
    return 0
  fi
  cat "$tmp_build_log" >&2
  return 1
}

if [ -n "$(git status --porcelain)" ]; then
  git diff --check
  run_build
  git add -A
  if ! git diff --cached --quiet; then
    git commit -m "$commit_message"
  fi
fi

git status --short --branch
git push -u origin HEAD

repo_name="$(gh repo view --json nameWithOwner --jq .nameWithOwner)"
pr_number="$(gh pr view --repo "$repo_name" --head "$branch" --json number --jq .number 2>/dev/null || true)"

if [ -z "$pr_body_file" ]; then
  tmp_body="$(mktemp)"
  cat >"$tmp_body" <<EOF
## 摘要

- 通过 Symphony Merging PR skill 合并分支 \`$branch\`。

## 验证

- \`git diff --check\`
- \`make build\`

Linear: ${branch}
EOF
  pr_body_file="$tmp_body"
fi

if [ -z "$pr_number" ]; then
  pr_url="$(gh pr create --repo "$repo_name" --base "$target" --head "$branch" --title "$pr_title" --body-file "$pr_body_file")"
  pr_number="${pr_url##*/}"
else
  gh pr edit "$pr_number" --repo "$repo_name" --title "$pr_title" --body-file "$pr_body_file"
fi

if ! gh label list --repo "$repo_name" --search symphony --limit 20 | grep -q '^symphony[[:space:]]'; then
  gh label create symphony --repo "$repo_name" --description "Symphony workflow" --color 5319E7
fi
gh pr edit "$pr_number" --repo "$repo_name" --add-label symphony

inline_comments="$(gh api "repos/$repo_name/pulls/$pr_number/comments" --jq 'length')"
review_decision="$(gh pr view "$pr_number" --repo "$repo_name" --json reviewDecision --jq '.reviewDecision // ""')"
mergeable="$(gh pr view "$pr_number" --repo "$repo_name" --json mergeable --jq '.mergeable // ""')"

if [ "$inline_comments" != "0" ]; then
  echo "PR has inline review comments; inspect before merge" >&2
  exit 1
fi
if [ "$review_decision" = "CHANGES_REQUESTED" ]; then
  echo "PR has requested changes; inspect before merge" >&2
  exit 1
fi
if [ "$mergeable" = "CONFLICTING" ]; then
  echo "PR is conflicting; resolve before merge" >&2
  exit 1
fi

checks_log="$(mktemp)"
if ! gh pr checks "$pr_number" --repo "$repo_name" --watch >"$checks_log" 2>&1; then
  if ! grep -q "no checks reported" "$checks_log"; then
    cat "$checks_log" >&2
    rm -f "$checks_log"
    exit 1
  fi
fi
rm -f "$checks_log"

gh pr merge "$pr_number" --repo "$repo_name" --squash --subject "$pr_title" --body-file "$pr_body_file"

git -C "$repo_root" fetch origin "$target"

merge_commit="$(gh pr view "$pr_number" --repo "$repo_name" --json mergeCommit --jq '.mergeCommit.oid // ""')"

cat <<EOF
PR: https://github.com/$repo_name/pull/$pr_number
merge_commit: $merge_commit
root_status: $(git -C "$repo_root" status --short --branch | head -1)
root_sync: skipped; repo-root checkout sync is owned by the orchestrator/operator, not the issue worktree agent
EOF
