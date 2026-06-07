#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

mkdir -p dist

GOOS="${GOOS:-$(go env GOOS)}"
GOARCH="${GOARCH:-$(go env GOARCH)}"

echo "==> servidor"
go build -o "dist/vassal-vlog-sync-server-${GOOS}-${GOARCH}" ./cmd/server

echo "==> cliente headless"
go build -tags headless -o "dist/vassal-vlog-sync-client-headless-${GOOS}-${GOARCH}" ./cmd/client

if command -v wails >/dev/null 2>&1; then
  echo "==> cliente wails (GUI)"
  (cd cmd/client && wails build -tags webkit2_41 -o "../../dist/vassal-vlog-sync-${GOOS}-${GOARCH}")
else
  echo "wails CLI não encontrado — pulando build GUI"
  echo "  instale: go install github.com/wailsapp/wails/v2/cmd/wails@latest"
  go build -tags "headless,webkit2_41" -o "dist/vassal-vlog-sync-gui-${GOOS}-${GOARCH}" ./cmd/client
fi

echo "Build concluído em dist/"
