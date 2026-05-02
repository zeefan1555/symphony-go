#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$repo_root"

go_cmd="${GO:-go}"
ldflags="${SYMPHONY_GO_BUILD_LDFLAGS:--linkmode=external}"
output="${SYMPHONY_GO_BINARY:-bin/symphony-go}"

mkdir -p "$(dirname "$output")"
exec "$go_cmd" build -ldflags="$ldflags" -o "$output" ./cmd/symphony-go
