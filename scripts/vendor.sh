#!/usr/bin/env bash
# Update vendored front-end libraries from the npm registry (no Node project needed).
set -euo pipefail
cd "$(dirname "$0")/.."
HTMX="${1:?htmx version}"; ALPINE="${2:?alpine version}"
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
( cd "$tmp" && npm pack "htmx.org@${HTMX}" "@alpinejs/csp@${ALPINE}" --silent >/dev/null
  mkdir h a && tar -xzf htmx.org-*.tgz -C h && tar -xzf alpinejs-csp-*.tgz -C a )
cp "$tmp/h/package/dist/htmx.min.js" static/vendor/htmx.min.js
cp "$tmp/a/package/dist/cdn.min.js" static/vendor/alpine-csp.min.js
sed -i.bak -E "s/\| htmx \| [0-9.]+ /| htmx | ${HTMX} /; s/\| ([^|]*Alpine[^|]*)\| [0-9.]+ /| \1| ${ALPINE} /" static/vendor/VERSIONS.md && rm -f static/vendor/VERSIONS.md.bak
echo "Updated htmx ${HTMX}, Alpine CSP ${ALPINE}. Run: make test-e2e"
