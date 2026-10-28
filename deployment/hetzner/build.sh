#!/usr/bin/env bash
set -euo pipefail
project_root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$project_root/frontend"
pnpm exec astro check
pnpm test
PUBLIC_DEMO_MODE=false pnpm build
cd "$project_root/backend"
go test ./...
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags='-s -w' -o bin/pbmanager-linux .
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags='-s -w' -o bin/pbmanager-agent-linux ./cmd/agent
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c -o bin/agent-tests-linux ./internal/agent
release_dir="${PBM_RELEASE_DIR:-/tmp}"
mkdir -p "$release_dir"
staging="$(mktemp -d "$release_dir/pbmanager-release.XXXXXX")"
trap 'rm -rf "$staging"' EXIT
mkdir -p "$staging/bin" "$staging/frontend" "$staging/deployment" "$staging/tests"
cp bin/pbmanager-linux "$staging/bin/pbmanager"
cp bin/pbmanager-agent-linux "$staging/bin/pbmanager-agent"
cp bin/agent-tests-linux "$staging/tests/agent-tests"
cp -R "$project_root/frontend/dist/." "$staging/frontend/"
cp "$project_root/deployment/hetzner/"*.service "$staging/deployment/"
tar -czf "$release_dir/pbmanager-release.tar.gz" -C "$staging" .
echo "$release_dir/pbmanager-release.tar.gz"
