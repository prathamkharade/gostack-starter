# CLAUDE.md – read this first

Go web app starter: **Go 1.27+ · Fiber v3 · templ · htmx 4 · Alpine.js (CSP build) · Tailwind v4 standalone · PostgreSQL 18**.
One binary, server-rendered HTML, no Node in production. Optimised for low hosting cost, EU-grade privacy and security.

## Commands (always use these, never improvise)
- `make dev` – Postgres+Mailpit in Docker, app with hot reload (http://localhost:8080, mail: :8025)
- `make check` – generate + lint + unit tests. **Run before saying a task is done.**
- `make test-integration` – DB tests (`//go:build integration`) · `make test-e2e` – Playwright (app must run)
- `make generate` – after editing `.templ` or CSS · `make migrate` · `make keygen`

## Map (where things live)
| Path | What |
|---|---|
| `cmd/app/main.go` | wiring: config → db → services → web/worker. Register new job handlers here |
| `internal/config` | ALL env settings (only place reading env). New setting → field + `.env.example` |
| `internal/web` | Fiber app, middleware (CSP, CSRF, rate limit, auth), core handlers, `routes()` |
| `internal/httpx` | handler helpers: `Render`, `IsPartial`, `Redirect`, `Toast`, `SetFlash`, `User`, `SetMeta`, `Query` |
| `internal/items` | **EXAMPLE feature module** (repo + handlers + views). Copy it for new resources |
| `internal/views/{layouts,components,pages}` | templ UI. Components catalog: `docs/ui.md` |
| `internal/listing` | search/filter/sort/pagination → safe SQL (`Spec`, `Parse`, `Build`, `Page`) |
| `internal/auth` | email-code/magic-link login, optional password, phone OTP, JWT+refresh rotation |
| `internal/privacy` | GDPR export/delete/retention. **Every resource registers an exporter** |
| `internal/security` | AES-GCM field encryption, blind index, argon2id, tokens, IP truncation |
| `internal/media`, `internal/storage` | upload sniffing, ffmpeg/vips compression, local or S3/R2 |
| `internal/jobs` | Postgres job queue (`Register`, `Enqueue`, `Every`) – no Redis |
| `internal/notify` | SMTP email + SMS (Twilio/Vonage/log) + email templates |
| `internal/seo` | meta, JSON-LD, sitemap, robots (AI crawler policy), llms.txt |
| `internal/database/migrations` | `NNNN_name.sql`, forward-only, embedded, auto-applied on start |
| `static/` | `css/input.css` (design tokens), `js/app.js` (Alpine components), vendored libs |
| `docs/` | deep docs – read only the one you need (index: `docs/README.md`) |

## Golden rules
1. **Personal data** (email, phone, name, address, free text about people, filenames) is stored encrypted:
   `keys.Encrypt(v, "table.column")` + `security.BlindIndex` if you must look it up. Classify every new
   field using `docs/compliance/data-catalog.md`. Never log PII, never put PII in URLs or audit `meta`.
2. **Every query** uses `$n` placeholders. Owner-scoped tables: `WHERE id = $1 AND owner_id = $2` (IDOR).
   Validate URL ids with `validate.IsUUID`. Return 404 (not 403) for other users' records.
3. **Every user-owned table**: `owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE` and a
   `privacy.RegisterExporter(...)` in its repo constructor.
4. **Strict CSP**: no inline `<script>`, no `onclick=`, no `style=""`. Behaviour → `Alpine.data()` in
   `static/js/app.js`; markup only references properties/method names (Alpine CSP build).
   Show/hide with `hidden x-bind:hidden="closed"` (a getter), never `x-show`: its inline style is copied by htmx swaps → CSP error.
5. **htmx 4, not 2**: inheritance is explicit (`hx-target:inherited`), events are `htmx:after:request` style,
   4xx responses ARE swapped (we use 422 + re-rendered form for validation errors). Details: `docs/reference/htmx4.md`.
6. **Handlers**: parse → validate (`internal/validate`) → service/repo → `httpx.Render` (fragment if
   `httpx.IsPartial(c)`) or `httpx.Redirect`. Errors: `return fiber.NewError(code, "human message")`.
7. **Styling**: use component classes (`btn btn-primary`, `card`, `input`, `badge`, `alert`, `table`) and
   semantic tokens (`bg-surface text-fg text-muted border-border`). Change the look only in `static/css/input.css`.
8. Accessibility (EU Accessibility Act, WCAG 2.2 AA): labels on every input, real `<button>`/`<a>`, focus visible,
   `aria-live` for dynamic results. e2e runs axe on pages.
9. Keep it boring: standard library first; no new dependency without a strong reason (write it in `docs/decisions/`).

## Adding things (use the skills)
- New app from an idea: `/new-app <idea>` · New resource/CRUD: `/add-resource <name: fields>` · Privacy review: `/compliance-review`
- New page: handler in `internal/web` (or feature pkg) + templ in `views/pages` + `httpx.SetMeta(c, seo.Meta{...})`.
- New background work: `queue.Register("kind", fn)` in `cmd/app/main.go`, `queue.Enqueue(ctx, "kind", payload)`.
- New migration: next number in `internal/database/migrations/`. Never edit an applied migration.

## Token-saving rules for AI sessions
- Don't read generated/vendor files (`*_templ.go`, `static/vendor`, `app.css`) – they are denied in settings.
- Start from the map above; open the closest existing example (`internal/items/*`) instead of exploring.
- Make small, verified steps: edit → `make check` → next. Ask before large refactors.

# Compact instructions
When compacting, keep: current task goal, files changed, failing test output, decisions made. Drop file dumps.
