# Testing

| Layer | Tool | Command | Where |
|---|---|---|---|
| Unit (pure logic) | `go test` | `make test` | `*_test.go` next to code (security, listing, storage SigV4 vectors, media, notify, seo, validate) |
| Integration (real Postgres + HTTP stack) | `go test -tags=integration` + Fiber `app.Test` | `make test-integration` | `internal/web/web_integration_test.go` |
| Browser journeys + accessibility + CSP | Playwright + axe-core | `make test-e2e` (app running via `make dev`) | `e2e/tests/*.spec.ts` (desktop + mobile) |
| Load | k6 | `k6 run loadtest/smoke.js` | `loadtest/` |
| Static analysis | gofmt, templ fmt, go vet, govulncheck | `make lint` | – |

`make check` = generate + lint + unit tests. CI runs lint + unit + integration on every push, then builds the image.

## Writing tests
- Unit: table-driven, no network, no DB.
- Integration: call `setup(t)`, use `e.do(method, path, form, cookies)`; read sign-in codes from the encrypted
  job payload (`e.lastEmailCode()`). Cover: happy path, validation (422), auth required (303), other user's
  record (404).
- E2E: one test per core journey using roles/labels (`getByRole`, `getByLabel`) – this also checks accessibility
  of names. Codes come from Mailpit's API.
- Bugs: write the failing test first, then fix.
