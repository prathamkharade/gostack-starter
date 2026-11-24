# Security model

| Threat (OWASP Top 10 2025) | Control in the kit |
|---|---|
| Broken access control / IDOR | owner-scoped SQL (`AND owner_id = $2`), 404 for foreign ids, `requireAuth`, `requireFresh`, `requireAdmin`, private media access check |
| Security misconfiguration | strict defaults: CSP, HSTS (prod), `X-Frame-Options DENY`, COOP, Permissions-Policy, no `Server` header, prod config validation (secure cookies, https) |
| Supply chain | few deps, pinned in go.mod, vendored front-end libs, govulncheck in CI, Dependabot, minimal images |
| Cryptographic failures | TLS 1.2+ via Caddy, AES-256-GCM field encryption, argon2id, HMAC blind indexes, `crypto/rand` everywhere |
| Injection (SQL/XSS/command) | pgx placeholders + `listing` whitelists; templ auto-escaping + URL sanitising; strict CSP without unsafe-inline/eval; ffmpeg/vips run with argv (no shell) |
| Insecure design | passwordless default, refresh-token reuse detection, OTP attempt limits, magic-link confirm page (scanner-safe), honeypot |
| Authentication failures | rate limits (global 300/min/IP, auth 10/5 min/IP, OTP 5/h/target), constant-time compares, dummy hash for unknown users, session list + "sign out everywhere" |
| Integrity failures | signed JWT with alg pinning (HS256 only), issuer+audience checks; CI builds images from source |
| Logging & alerting failures | structured JSON logs, request ids, audit log of security events, slow-query log |
| Exceptional conditions | recover middleware, timeouts (HTTP, DB statement 15 s, jobs), graceful shutdown |

## CSRF
Tokenless: Fiber `NewCrossOriginProtection` (same algorithm as Go's `net/http.CrossOriginProtection`) rejects
unsafe methods unless `Sec-Fetch-Site` is same-origin/none or `Origin` matches. Cookies are `SameSite=Lax`
and `__Host-` prefixed. All state changes use POST.

## Uploads
Magic-byte sniffing (ignore client MIME/extension), executables blocked, HTML downgraded to text, SVG never
inline, metadata stripped, served with `nosniff`, `Content-Disposition`, `CSP: sandbox`. Consider ClamAV (as
a job) if users share files with each other.

## Containers & server
Non-root user, read-only root FS, `cap_drop: ALL`, `no-new-privileges`, memory limits, DB not exposed,
SSH keys only, ufw, unattended upgrades, log rotation.

## Secrets
Only in `.env` / environment. `.env` is git-ignored and denied to AI tools. Rotate on staff changes.

## Things to add per app
2FA (TOTP/passkeys via WebAuthn) for admin roles, admin action approval, WAF rules at Cloudflare, per-tenant
isolation tests for B2B, penetration test before handling sensitive data.
