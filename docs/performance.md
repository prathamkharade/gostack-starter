# Performance budget & techniques

**Budget** (mid-range phone, 4G): LCP < 1.5 s, INP < 100 ms, CLS < 0.05, first page ≤ 50 KB compressed,
server p95 < 50 ms for HTML.

Baseline of the kit: CSS ~6 KB gzip, htmx ~13 KB gzip, Alpine CSP ~24 KB gzip, app.js ~1 KB, system fonts (0 KB).

## Techniques already in place
- Server-rendered HTML (no hydration, no client router). Boosted navigation swaps the body without reloading assets.
- Fingerprinted static assets embedded in the binary, `Cache-Control: max-age=31536000`.
- zstd/gzip/HTTP3 at Caddy. Scripts `defer`red; only theme.js (300 B) blocks to prevent theme flash.
- Images: WebP/AVIF, `srcset` widths, `loading="lazy"`, explicit width/height (no layout shift), `fetchpriority=high` for LCP.
- Video: H.264/AV1 at ≤ 720p, `faststart`, poster, `preload="metadata"`, byte-range support.
- DB: pgx pool sized small, indexes per query, GIN full-text & trigram indexes, capped counts
  (`CountLimit`) and `MaxOffset` to keep list queries O(page).
- Public pages send `Cache-Control: public, max-age=300` (CDN-friendly); signed-in pages `private, no-store`.

## Rules when adding features
- Every new query: check `EXPLAIN ANALYZE` on realistic data; add the index in the same migration.
- No N+1: fetch related rows with JOIN or `WHERE id = ANY($1)`.
- Heavy work (exports, emails, image processing, webhooks) → background job.
- No new client JS library without measuring it. Prefer htmx attributes or a tiny Alpine component.
- Measure: Lighthouse in Chrome DevTools, `k6 run loadtest/smoke.js`, Postgres slow-query log (≥ 500 ms).
