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

generated_root="$repo_root/internal/generated/hertz"
# Generated scaffold models live under internal/generated/hertz/scaffold.
rm -rf "$generated_root/scaffold"

for idl in \
  idl/scaffold/orchestrator.thrift \
  idl/scaffold/workspace.thrift \
  idl/scaffold/codex_session.thrift \
  idl/scaffold/workflow.thrift
do
  hz model \
    --idl "$repo_root/$idl" \
    --module github.com/zeefan1555/symphony-go \
    --out_dir "$repo_root" \
    --model_dir internal/generated/hertz
done

"$repo_root/scripts/check_generated_hertz_boundary.sh"
