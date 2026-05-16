#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "usage: $0 <binary> <workflow> [symphony-go run flags...]" >&2
  exit 2
fi

binary="$1"
workflow="$2"
shift 2

workflow_dir="$(cd "$(dirname "$workflow")" && pwd -P)"
workflow_base="$(basename "$workflow")"

workflow_cwd="$(
  awk '
    /^workspace:[[:space:]]*$/ { in_workspace = 1; next }
    in_workspace && /^[^[:space:]]/ { in_workspace = 0 }
    in_workspace && /^[[:space:]]+cwd:[[:space:]]*/ {
      sub(/^[[:space:]]+cwd:[[:space:]]*/, "")
      gsub(/^["'\'']|["'\'']$/, "")
      print
      exit
    }
  ' "$workflow"
)"

if [[ -n "$workflow_cwd" ]]; then
  if [[ "$workflow_cwd" = /* ]]; then
    label="$(basename "$workflow_cwd")"
  else
    label="$(basename "$(cd "$workflow_dir/$workflow_cwd" && pwd -P)")"
  fi
elif [[ "$workflow_base" == WORKFLOW-*.md ]]; then
  label="${workflow_base#WORKFLOW-}"
  label="${label%.md}"
elif [[ "$workflow_base" == "WORKFLOW.md" || "$workflow_base" == "workflow.md" ]]; then
  label="$(basename "$workflow_dir")"
else
  label="${workflow_base%.md}"
fi

alias_dir="$(dirname "$binary")/process-names"
alias_binary="$alias_dir/symphony-go($label)"
mkdir -p "$alias_dir"
cp "$binary" "$alias_binary"

exec "$alias_binary" run --workflow "$workflow" "$@"
