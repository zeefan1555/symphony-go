#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
generated_root="$repo_root/internal/generated/hertz"

if [ ! -d "$generated_root" ]; then
  echo "generated Hertz root does not exist: internal/generated/hertz" >&2
  exit 1
fi

while IFS= read -r file; do
  if ! head -n 1 "$file" | grep -Eq '^// Code generated '; then
    echo "non-generated Go file in generated Hertz tree: ${file#$repo_root/}" >&2
    exit 1
  fi

  if grep -Eq 'internal/(orchestrator|workspace|codex|workflow|linear)' "$file"; then
    echo "generated Hertz file imports business internals: ${file#$repo_root/}" >&2
    exit 1
  fi
done < <(find "$generated_root" -type f -name '*.go' | sort)
