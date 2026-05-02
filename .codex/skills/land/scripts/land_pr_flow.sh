#!/usr/bin/env bash
set -euo pipefail

repo_root="/Users/bytedance/symphony-go"
target="main"

usage() {
  cat <<'EOF'
Usage: land_pr_flow.sh [--repo-root PATH] [--target BRANCH]

Land an existing PR from the current issue worktree branch, then fast-forward
the root checkout. This script does not create PRs and does not remove the issue
worktree; Symphony Go cleans the worktree after the Merging skill succeeds.
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

if [ -n "$(git status --porcelain)" ]; then
  echo "land requires a clean issue worktree; create/update the PR before Merging" >&2
  git status --short --branch >&2
  exit 1
fi

if ! command -v gh >/dev/null 2>&1; then
  echo "missing gh CLI" >&2
  exit 1
fi

repo_name="$(gh repo view --json nameWithOwner --jq .nameWithOwner)"
pr_number="$(gh pr view "$branch" --repo "$repo_name" --json number --jq .number 2>/dev/null || true)"
if [ -z "$pr_number" ]; then
  echo "no open PR found for branch $branch; Merging expects PR creation to happen before Human Review" >&2
  exit 1
fi

pr_state="$(gh pr view "$pr_number" --repo "$repo_name" --json state --jq .state)"
if [ "$pr_state" != "OPEN" ]; then
  echo "PR #$pr_number is $pr_state, expected OPEN" >&2
  exit 1
fi

inline_comments="$(gh api "repos/$repo_name/pulls/$pr_number/comments" --jq 'length')"
review_decision="$(gh pr view "$pr_number" --repo "$repo_name" --json reviewDecision --jq '.reviewDecision // ""')"
mergeable="$(gh pr view "$pr_number" --repo "$repo_name" --json mergeable --jq '.mergeable // ""')"

if [ "$inline_comments" != "0" ]; then
  echo "PR #$pr_number has inline review comments; resolve or push back before merge" >&2
  exit 1
fi
if [ "$review_decision" = "CHANGES_REQUESTED" ]; then
  echo "PR #$pr_number has requested changes; resolve before merge" >&2
  exit 1
fi
if [ "$mergeable" = "CONFLICTING" ]; then
  echo "PR #$pr_number is conflicting; update the branch before merge" >&2
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

pr_title="$(gh pr view "$pr_number" --repo "$repo_name" --json title --jq .title)"
pr_head="$(gh pr view "$pr_number" --repo "$repo_name" --json headRefOid --jq .headRefOid)"
tmp_body="$(mktemp)"
tmp_origin_file="$(mktemp)"
cleanup() {
  rm -f "$tmp_body" "$tmp_origin_file"
}
trap cleanup EXIT

gh pr view "$pr_number" --repo "$repo_name" --json body --jq '.body // ""' >"$tmp_body"
gh pr merge "$pr_number" --repo "$repo_name" --squash --subject "$pr_title" --body-file "$tmp_body" --match-head-commit "$pr_head"

git -C "$repo_root" fetch origin "$target"

clear_untracked_path() {
  local rel="$1"
  local abs="$repo_root/$rel"
  if [ -d "$abs" ]; then
    while IFS= read -r -d '' file; do
      local child="${file#"$repo_root"/}"
      if git -C "$repo_root" show "origin/$target:$child" >"$tmp_origin_file" 2>/dev/null && cmp -s "$file" "$tmp_origin_file"; then
        rm -f "$file"
      else
        echo "root checkout has divergent untracked file: $child" >&2
        exit 1
      fi
    done < <(find "$abs" -type f -print0)
    find "$abs" -type d -empty -delete 2>/dev/null || true
  else
    if git -C "$repo_root" show "origin/$target:$rel" >"$tmp_origin_file" 2>/dev/null && cmp -s "$abs" "$tmp_origin_file"; then
      rm -f "$abs"
    else
      echo "root checkout has divergent untracked file: $rel" >&2
      exit 1
    fi
  fi
}

while IFS= read -r line; do
  [ -z "$line" ] && continue
  status="${line:0:2}"
  path="${line:3}"
  case "$status" in
    '??')
      clear_untracked_path "$path"
      ;;
    *)
      if git -C "$repo_root" diff --quiet "origin/$target" -- "$path"; then
        git -C "$repo_root" restore --staged --worktree -- "$path"
      else
        echo "root checkout has divergent local edit: $path" >&2
        exit 1
      fi
      ;;
  esac
done < <(git -C "$repo_root" status --porcelain)

git -C "$repo_root" switch "$target"
git -C "$repo_root" pull --ff-only origin "$target"

merge_commit="$(gh pr view "$pr_number" --repo "$repo_name" --json mergeCommit --jq '.mergeCommit.oid // ""')"

cat <<EOF
PR: https://github.com/$repo_name/pull/$pr_number
merge_commit: $merge_commit
root_status: $(git -C "$repo_root" status --short --branch | head -1)
worktree_cleanup: owned by orchestrator
EOF
