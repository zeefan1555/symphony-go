#!/usr/bin/env bash
set -euo pipefail

workspace="${1:-$(pwd -P)}"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd "$script_dir/.." && pwd -P)"
issue="$(basename "$workspace")"
branch="symphony-go/$issue"
remote_url="${SYMPHONY_REMOTE_URL:-git@github.com:zeefan1555/symphony-go.git}"

cd "$repo_root"
rm -rf "$workspace"

if git show-ref --verify --quiet "refs/heads/$branch"; then
  git worktree add "$workspace" "$branch"
else
  git worktree add -b "$branch" "$workspace" HEAD
fi

git -C "$workspace" remote set-url origin "$remote_url"
