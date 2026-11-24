# Vendored front-end libraries

Self-hosted (no CDN): faster (same connection, HTTP/2), works offline, no third-party
requests that leak visitor IPs (GDPR), and allowed by `script-src 'self'`.

| File | Library | Version | License |
|---|---|---|---|
| htmx.min.js | htmx | 4.0.2 | 0BSD (htmx.LICENSE) |
| alpine-csp.min.js | @alpinejs/csp (Alpine.js CSP build, no eval) | 3.17.5 | MIT (alpine.LICENSE) |

Upgrade: `make vendor HTMX=4.x.y ALPINE=3.x.y` (downloads from the npm registry), then run `make test-e2e`.
