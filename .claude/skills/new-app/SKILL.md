---
name: new-app
description: Turn a plain-language app idea into a working application on this starter kit (spec, data model with privacy classification, resources, pages, tests).
argument-hint: "<describe your app idea>"
disable-model-invocation: true
---

You are turning this starter kit into the app described here: $ARGUMENTS

Work in phases. Finish each phase, run `make check`, show a short summary, then continue.
Be token-efficient: read CLAUDE.md, then only the files you need (start from `internal/items/*` as the pattern).

## Phase 0 – Clarify (max 5 questions, one message)
Ask only what changes the data model or legal obligations: who the users/roles are, what they create,
payments?, children under 16?, health/financial/biometric data?, which countries. If the user already
answered or says "decide", choose sensible defaults and state them.

## Phase 1 – Spec (write `docs/product.md`, ≤ 1 page)
- One-sentence pitch, target users, roles.
- Resources (nouns) with fields, relations, who can see/edit each.
- User journeys (sign up → core action → value) and pages list with URL, public/private, indexable or not.
- Out of scope for v1.

## Phase 2 – Data & compliance (write `docs/compliance/app-data-map.md`)
For EVERY field: classification (public / internal / personal / sensitive-special-category / financial),
lawful basis, retention, storage method (plain, encrypted `_enc`, blind index `_bidx`, hashed), who can access.
Use `docs/compliance/data-catalog.md`. Flag anything that needs a DPIA, age gate, or explicit consent.
Update the table in `internal/views/pages/legal.templ` (Privacy) to match. Never collect what the spec doesn't need.

## Phase 3 – Branding & shell
- `.env.example`: APP_NAME, SITE_DESCRIPTION, ORG_*; brand colours in `static/css/input.css` (@theme brand-*).
- `components.MainNav` entries; home page copy in `views/pages/home.templ` (what / who / how, plain sentences – GEO).
- Logo/favicon in `static/img/`.

## Phase 4 – Resources (repeat per resource, smallest first)
Follow `.claude/skills/add-resource/SKILL.md` exactly: migration → repo (+ exporter) → handlers → views →
routes → integration test. Run `make check` after each resource.

## Phase 5 – Polish
- Empty states, validation messages, toasts, mobile layout check, `seo.Meta` + JSON-LD on public pages,
  sitemap entries for public content (`handlers_public.go`), robots disallow for private paths.
- If the example `items` module is no longer needed: delete `internal/items`, migration `0002_items.sql`
  (only if never deployed – otherwise add a migration that drops the table), the `items.Routes` call,
  `Items` deps in `web.Deps`/`main.go`, and sitemap usage.

## Phase 6 – Verify
`make check`, `make test-integration`, start `make dev` and run `make test-e2e`; extend `e2e/tests/journey.spec.ts`
with the app's main journey. Report what was built, what is stubbed, and the launch checklist items from
`docs/compliance/checklist.md` the owner must still do (legal texts, DPAs, etc.).
