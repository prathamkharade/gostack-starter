# ADR 0001 – Stack and architecture

**Context.** Goal: build many different apps quickly, host them for a few euros a month, keep maintenance
low, meet EU privacy/security/accessibility rules, and let AI agents work on the code cheaply.

**Decision.**
- Go + Fiber v3 (fast, one static binary), templ (type-checked HTML), htmx 4 + Alpine CSP build (interactivity
  without a SPA or Node runtime), Tailwind v4 standalone CLI (no npm in the build), PostgreSQL 18 for data,
  queue and search, Caddy for TLS. Docker Compose for dev parity and production.
- Tokenless CSRF (Sec-Fetch-Site), strict CSP (no eval/inline), field-level encryption of personal data.
- Passwordless email sign-in by default; JWT access + rotating refresh tokens.
- Self-written small components where libraries would add churn: S3 SigV4 client, migrator, validator, job queue.
- Forward-only SQL migrations; feature-folder packages; one example module to copy.

**Consequences.** Very low RAM/CPU and cost; few moving parts; one person can operate it. Trade-offs: no
React ecosystem; htmx 4 is new (docs in `docs/reference/htmx4.md`); Alpine CSP build restricts inline
expressions; Fiber is fasthttp-based (not net/http) – use Fiber middleware or its adaptor for net/http packages.
