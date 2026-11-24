---
name: compliance-review
description: Review recent changes (or the whole app) for privacy, security and accessibility compliance gaps against this kit's rules. Use before releases or after adding data fields.
argument-hint: "[git range or path, default: uncommitted changes]"
---

Scope: $ARGUMENTS (default: `git diff` + untracked files). Read only changed files plus the docs referenced below.

Check and report as a table (issue · file:line · rule · fix). Fix trivial issues directly, ask before large ones.

**Privacy (docs/compliance/data-storage.md, data-catalog.md)**
- New personal-data fields stored encrypted (`_enc`, aad = table.column)? Lookups via blind index only?
- Data minimisation: is each field needed for the stated purpose? Retention defined?
- Owner-linked tables use `ON DELETE CASCADE` and register a `privacy.RegisterExporter`?
- No PII in logs, URLs, audit `meta`, job kinds, error messages, analytics.
- Privacy notice table (`views/pages/legal.templ`) and `docs/compliance/app-data-map.md` updated?
- New third-party service = new processor: DPA, location, transfer safeguard noted in docs?

**Security (docs/security.md)**
- Parameterised SQL only; owner scoping on every read/write; `validate.IsUUID` for path ids.
- Input validation server-side; output only through templ (no `templ.Raw` with user data).
- No inline JS/CSS (CSP); no new external origins without updating CSP in `internal/web/server.go`.
- Sensitive actions use `requireFresh`; new auth-like endpoints are rate limited.
- Secrets only via config/env; nothing secret committed.

**Accessibility (docs/ui.md)**
- Inputs have labels, errors are linked with aria-describedby, buttons are `<button>`, dynamic regions `aria-live`.

Finish with `make check` and a one-paragraph verdict.
