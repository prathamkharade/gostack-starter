# Architecture

```
Browser ──HTTPS/HTTP3──> Caddy (TLS, compression, body limits)
                           │
                           ▼
                 app (one Go binary, mode: all | web | worker)
   ┌──────────────────────────────────────────────────────────────┐
   │ Fiber: recover → static → requestid → security headers (CSP) │
   │  → access log → cross-origin CSRF → rate limit → view data  │
   │  → authenticate (JWT cookie / Bearer, refresh rotation)      │
   │                                                              │
   │ handlers (internal/web, internal/<feature>)                  │
   │   └─ templ views → HTML (full page or htmx fragment)         │
   │ services: auth, privacy, media, repos (pgx)                  │
   │ jobs worker: email.send, sms.send, media.process,            │
   │              storage.delete, retention (hourly)              │
   └──────────────┬───────────────────────────┬───────────────────┘
                  ▼                           ▼
          PostgreSQL 18                Storage: local volume
   (data, sessions, jobs queue,        or S3-compatible (R2/B2)
    full-text search, audit)
```

## Request lifecycle
1. Static assets (`/static/*`) are served from the embedded FS with 1-year cache (fingerprinted URLs).
2. Middleware sets CSP and security headers, rejects cross-site unsafe requests (Sec-Fetch-Site/Origin),
   applies the per-IP limiter, creates `httpx.ViewData`, reads the flash cookie, and resolves the user.
3. Handler parses + validates input, calls a repo/service, then renders:
   - normal/boosted request → full page (`layouts.Base` + content)
   - htmx partial request (`HX-Request-Type: partial`) → only the fragment (e.g. `Results`)
   - success after POST → `httpx.Redirect` (303, or `HX-Location` for htmx) + flash/toast
   - validation error → 422 + the form re-rendered with messages (htmx 4 swaps 4xx)
4. Errors → `errorHandler`: friendly page, or toast + `HX-Reswap: none` for htmx.

## Progressive enhancement
Everything works without JavaScript (plain links and forms). `hx-boost:inherited` on `<body>` turns links and
forms into fetch + body swap (no full reloads). Specific widgets (search-as-you-type, inline delete, login
steps, upload polling) use explicit `hx-*` attributes. Alpine adds small client-side state (menus, dialogs,
toasts, theme).

## Auth model
- Sign-in = email code or magic link (same flow creates the account) or optional password.
- Access token: HS256 JWT, 15 min, HttpOnly `__Host-at` cookie (or `Authorization: Bearer` for API/mobile).
- Refresh token: 256-bit random, 30 days, `__Host-rt`, stored as SHA-256 in `sessions`; rotated on every use;
  reuse after the 10 s grace window revokes the whole family and emails the user.
- Sensitive actions (`requireFresh`) re-check the session row so revocation is immediate.

## Background jobs
`jobs` table + `FOR UPDATE SKIP LOCKED`; payload AES-encrypted and wiped on success; retries with exponential
backoff (30 s → 6 h, 8 attempts); handlers must be idempotent. Periodic tasks via `queue.Every`.

## Media pipeline
Upload → stream to temp file (size limit, SHA-256) → sniff magic bytes → block executables → per-owner dedupe →
store original → `media.process` job → ffmpeg/vips renditions (WebP/AVIF widths, H.264/AV1 MP4 + poster,
AAC audio, gzip text) with metadata stripped → upload variants → delete original. See `media.md`.

## Modes & scaling
`app` (default) runs web + worker in one process – cheapest. `app web` and `app worker` split them; both are
stateless and can be replicated. Migrations run at start behind an advisory lock.

## Package dependency rules
`cmd/app` → `web` → feature packages (`items`…) → `httpx`, `views/*`, services → `security`, `listing`,
`validate`, `storage` (leaf packages, stdlib only). Feature packages never import `web`.
