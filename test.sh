#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$repo_root"

if [[ $# -eq 0 ]]; then
	set -- ./...
fi

go_cmd="${GO:-go}"
ldflags="${SYMPHONY_GO_TEST_LDFLAGS:--linkmode=external}"

exec "$go_cmd" test -ldflags="$ldflags" "$@"
