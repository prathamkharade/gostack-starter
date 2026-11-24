#!/usr/bin/env bash
# First-time setup. Safe to re-run.
set -euo pipefail
cd "$(dirname "$0")/.."

TW_VERSION="${1:-4.3.5}"

command -v go >/dev/null || { echo "Go is not installed. See docs/setup-windows.md"; exit 1; }

echo "==> Tailwind CSS standalone CLI v${TW_VERSION}"
mkdir -p bin
if [ ! -x bin/tailwindcss ] || ! bin/tailwindcss --help 2>/dev/null | grep -q "v${TW_VERSION}"; then
  case "$(uname -s)-$(uname -m)" in
    Linux-x86_64)  asset=tailwindcss-linux-x64 ;;
    Linux-aarch64) asset=tailwindcss-linux-arm64 ;;
    Darwin-arm64)  asset=tailwindcss-macos-arm64 ;;
    Darwin-x86_64) asset=tailwindcss-macos-x64 ;;
    *) echo "Unsupported platform $(uname -s)-$(uname -m). On Windows use WSL2."; exit 1 ;;
  esac
  curl -fsSL -o bin/tailwindcss "https://github.com/tailwindlabs/tailwindcss/releases/download/v${TW_VERSION}/${asset}"
  chmod +x bin/tailwindcss
fi

echo "==> Go modules and tools (templ, air, govulncheck are pinned in go.mod)"
go mod tidy
go mod download

if [ ! -f .env ]; then
  echo "==> Creating .env with fresh secrets"
  cp .env.example .env
  secrets="$(go run ./cmd/app keygen)"
  # split on the FIRST "=" only: `IFS='=' read key value` would drop base64 "=" padding
  while IFS= read -r line; do
    [ -n "$line" ] || continue
    key="${line%%=*}"; value="${line#*=}"
    # replace the empty placeholder line for this key
    sed -i.bak "s|^${key}=.*|${key}=${value}|" .env
  done <<< "$secrets"
  # dev Postgres from compose.yaml listens on DB_PORT (default 5433)
  dburl="postgres://app:app@localhost:${DB_PORT:-5433}/app?sslmode=disable"
  if grep -q '^DATABASE_URL=' .env; then
    sed -i.bak "s|^DATABASE_URL=.*|DATABASE_URL=${dburl}|" .env
  else
    echo "DATABASE_URL=${dburl}" >> .env
  fi
  rm -f .env.bak
else
  echo "==> .env exists, leaving it alone"
fi

echo "==> Formatting + generating templates and CSS"
go tool templ fmt .
go tool templ generate
bin/tailwindcss -i static/css/input.css -o static/css/app.css --minify

echo
echo "Done. Commit go.mod + go.sum. Next:  make dev  then open http://localhost:8080 (emails: http://localhost:8025)"
