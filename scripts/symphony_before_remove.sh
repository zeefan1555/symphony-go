#!/usr/bin/env bash
set -euo pipefail

workspace="${1:-$(pwd -P)}"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd "$script_dir/.." && pwd -P)"

cd "$repo_root"
git worktree remove --force "$workspace"
