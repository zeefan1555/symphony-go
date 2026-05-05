#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

require_dir() {
  local root="$1"
  local label="$2"
  if [ ! -d "$root" ]; then
    echo "$label does not exist: ${root#$repo_root/}" >&2
    exit 1
  fi
}

check_generated_tree() {
  local root="$1"
  local label="$2"
  local forbid_business_imports="${3:-yes}"
  local found=0

  require_dir "$root" "$label"
  while IFS= read -r file; do
    found=1
    if ! head -n 1 "$file" | grep -Eq '^// Code generated '; then
      echo "non-generated Go file in $label: ${file#$repo_root/}" >&2
      exit 1
    fi

    if [ "$forbid_business_imports" = "yes" ] &&
      grep -Eq 'internal/(orchestrator|workspace|codex|workflow|issuetracker|service)' "$file"; then
      echo "generated Hertz file imports business internals: ${file#$repo_root/}" >&2
      exit 1
    fi
  done < <(find "$root" -type f -name '*.go' | sort)

  if [ "$found" -eq 0 ]; then
    echo "$label contains no generated Go files: ${root#$repo_root/}" >&2
    exit 1
  fi
}

check_internal_service_boundary() {
  local root="$repo_root/internal/service"

  require_dir "$root" "internal service root"
  while IFS= read -r file; do
    if grep -Eq 'github.com/cloudwego/hertz/pkg/app|app\.RequestContext' "$file"; then
      echo "internal service imports Hertz RequestContext boundary: ${file#$repo_root/}" >&2
      exit 1
    fi

    if grep -Eq 'internal/issuetracker' "$file"; then
      echo "internal service imports issue tracker integration: ${file#$repo_root/}" >&2
      exit 1
    fi
  done < <(find "$root" -type f -name '*.go' | sort)
}

check_generated_tree "$repo_root/biz/handler" "biz handler shell" yes
check_generated_tree "$repo_root/biz/model" "biz model shell" yes
check_generated_tree "$repo_root/biz/router" "biz router shell" yes
check_internal_service_boundary
