#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="$(go env GOPATH)/bin:${PATH}"

if ! command -v hz >/dev/null 2>&1; then
  echo "hz not found. Install with: go install -ldflags=-linkmode=external github.com/cloudwego/hertz/cmd/hz@v0.9.7" >&2
  exit 1
fi

if ! command -v thriftgo >/dev/null 2>&1; then
  echo "thriftgo not found. Install with: go install github.com/cloudwego/thriftgo@latest" >&2
  exit 1
fi

mkdir -p "$repo_root/internal/generated/hertz/control"
cd "$repo_root/internal/generated/hertz/control"

hz new \
  --force \
  --handler_dir handler \
  --model_dir model \
  --router_dir router \
  --idl "$repo_root/idl/control/http.thrift"

perl -pi -e 's/[ \t]+$//' .gitignore
