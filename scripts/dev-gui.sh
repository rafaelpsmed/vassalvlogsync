#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export PATH="${HOME}/go/bin:${PATH}"

if ! command -v wails >/dev/null 2>&1; then
  echo "Wails CLI não encontrado. Instale com:"
  echo "  go install github.com/wailsapp/wails/v2/cmd/wails@latest"
  exit 1
fi

cd "$ROOT/cmd/client"
exec wails dev -tags webkit2_41 "$@"
