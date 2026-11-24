---
name: add-resource
description: Add a new database-backed resource (CRUD + search/filter/pagination + privacy export) following the items example module. Use when the user asks for a new entity, model, table or CRUD screen.
argument-hint: "<name>: <field:type,...>  e.g. projects: name:text!, status:enum(active,done), due:date, notes:text"
---

Create resource: $ARGUMENTS

Copy the pattern of `internal/items` (read `repo.go`, `handlers.go`, `views.templ` once; do not explore further).
Use singular Go type (`Project`), plural package/table/URL (`projects`).

1. **Migration** `internal/database/migrations/NNNN_<plural>.sql` (next number):
   - `id uuid PRIMARY KEY DEFAULT uuidv7()`, `owner_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE`,
     `created_at`/`updated_at timestamptz NOT NULL DEFAULT now()`.
   - CHECK constraints for lengths/enums. Index `(owner_id, created_at DESC, id DESC)`.
   - Searchable text → generated `search tsvector` + GIN; fuzzy title → `gin_trgm_ops` index.
   - Personal data fields → `bytea` `_enc` columns (encrypt in repo with aad `"<table>.<column>"`),
     plus `_bidx bytea` only if you must look it up by value. Classify per `docs/compliance/data-catalog.md`.
2. **Repo** `internal/<plural>/repo.go`: struct, `listing.Spec` (only whitelisted filters/sorts),
   `List/Get/Create/Update/Delete` (Update/Delete scoped by owner_id), and in `NewRepo`
   `privacy.RegisterExporter("<plural>", ...)`.
3. **Handlers** `internal/<plural>/handlers.go`: `Routes(app, repo, ..., requireAuth)`; validate with
   `internal/validate`; `validate.IsUUID` on path ids; 422 + re-rendered form on errors; `httpx.Toast` + `httpx.Redirect` on success;
   `httpx.IsPartial` → render `Results` fragment for list requests.
4. **Views** `internal/<plural>/views.templ`: ListPage (PageHeader + ListControls + Results), Results
   (table/cards + Pagination + EmptyState), FormPage, optional PublicPage (+ `seo.Meta`, JSON-LD, sitemap).
   Only use components from `internal/views/components` and classes from `docs/ui.md`.
5. **Wire**: add repo to `web.Deps` + `Server`, call `<plural>.Routes(...)` in `internal/web/server.go`,
   construct in `cmd/app/main.go`, add a `components.MainNav` entry, add private paths to robots disallow.
6. **Test**: add a case to `internal/web/web_integration_test.go` (create → list/search → update → delete,
   and that another user gets 404). Run `make check` and `make test-integration`.

Done = all of the above + `make check` green. Report the files created.
