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

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

hz new \
  --module github.com/zeefan1555/symphony-go \
  --out_dir "$tmp_dir" \
  --handler_dir biz/handler \
  --model_dir biz/model \
  --idl "$repo_root/idl/control/http.thrift"

find "$tmp_dir/biz/router" -type f -name '*.go' -exec perl -pi -e 's/^package Http$/package http/' {} +

rm -rf "$repo_root/biz/handler" "$repo_root/biz/model" "$repo_root/biz/router/control"
mkdir -p "$repo_root/biz/router"
cp -R "$tmp_dir/biz/handler" "$repo_root/biz/handler"
cp -R "$tmp_dir/biz/model" "$repo_root/biz/model"
cp -R "$tmp_dir/biz/router/control" "$repo_root/biz/router/control"

find "$repo_root/biz" -type f -name '*.go' -exec perl -pi -e 's/[ \t]+$//' {} +
find "$repo_root/biz" -type f -name '*.go' -exec chmod 0644 {} +
