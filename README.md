# Go + htmx starter kit

A production-ready base for **any** web app: describe your idea, and build on top of auth, privacy, media,
search, SEO, testing and deployment that are already done.

**Stack:** Go 1.27+ · Fiber v3 · templ · htmx 4 · Alpine.js (CSP build) · Tailwind CSS v4 (standalone CLI) ·
PostgreSQL 18 · Caddy · Docker Compose. One binary, no Node in production, ~€6–10/month to host.

## What you get
- **Frictionless sign-in**: email code or magic link (sign-up = sign-in), optional password, optional SMS phone
  verification, rotating refresh tokens with theft detection, "sign out everywhere".
- **EU-grade privacy**: personal data encrypted per field (AES-256-GCM + blind indexes), consent records,
  one-click data export & account deletion, automatic retention, no third-party trackers → no cookie banner.
- **Security**: strict CSP (no inline/eval), tokenless CSRF, rate limits, argon2id, hardened containers, encrypted backups.
- **UI kit**: accessible components (forms, tables, dialogs, toasts, pagination, media), dark mode, design tokens in one file.
- **Lists**: search-as-you-type (Postgres full-text + fuzzy), whitelisted filters, sorting, pagination, URL state.
- **Media**: any file type; images → WebP/AVIF responsive sizes, video → H.264/AV1 720p + poster, audio → AAC,
  text gzip, metadata stripped, de-duplicated; local disk or S3/R2.
- **Email/SMS**: any SMTP provider (SES, Brevo, Resend, Postmark) or Twilio/Vonage – just fill `.env`.
- **SEO/GEO**: meta & Open Graph, JSON-LD, sitemap, robots with AI-crawler policy, llms.txt, security.txt.
- **Background jobs** in Postgres (no Redis). **Tests**: unit, integration, Playwright + axe, k6.
- **AI-ready**: `CLAUDE.md`, focused docs, and Claude Code skills (`/new-app`, `/add-resource`, `/compliance-review`).

## Quick start (Linux / macOS / Windows via WSL2)
```bash
make setup   # tools, deps, .env with fresh secrets
make dev     # Postgres + Mailpit in Docker, app with hot reload
```
Open http://localhost:8080 · dev inbox http://localhost:8025 · `make help` for all commands.
Windows without Docker Desktop: [docs/setup-windows.md](docs/setup-windows.md).

## Start your own app
Copy this folder into a new git repo, open it in VS Code with Claude Code, and run:
```
/new-app <describe your idea>
```
See [docs/claude-code-workflow.md](docs/claude-code-workflow.md) for the full prompt and daily workflow.

## Deploy
[docs/hosting.md](docs/hosting.md) – one VPS, Caddy auto-HTTPS, GitHub Actions → GHCR → SSH deploy, daily encrypted backups.

## Docs
[docs/README.md](docs/README.md). Compliance: [docs/compliance/](docs/compliance/README.md) (not legal advice).

## Licence
Your code: your choice. Vendored: htmx (0BSD), Alpine.js (MIT) – see `static/vendor/`.
