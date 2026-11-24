# Search, filters, sorting, pagination

One declarative `listing.Spec` per resource → safe SQL + UI controls. No search server needed: Postgres
full-text search (GIN) + trigram (`pg_trgm`) handle millions of rows on a small VPS.

```go
var Spec = listing.Spec{
    From: "items", Select: "id, owner_id, title, body, status, cover_id, created_at, updated_at",
    SearchFTS: "search", SearchLike: []string{"title"},          // full-text + fuzzy substring
    Filters: []listing.Filter{
        {Param: "status", Column: "status", Op: listing.In, Kind: listing.Enum, Values: Statuses, Label: "Status"},
        {Param: "from", Column: "created_at", Op: listing.Gte, Kind: listing.Date, Label: "From"},
    },
    Sorts: map[string]string{"newest": "created_at DESC, id DESC", "title": "title ASC, id ASC"},
    DefaultSort: "newest", DefaultPerPage: 20, CountLimit: 10_000,
}

p := Spec.Parse(httpx.Query(c))                       // invalid values dropped silently
q, args, cq, cargs := Spec.Build(p, "owner_id = $1", userID)
// query rows → listing.NewPage(rows, p, total, Spec.CountLimit)
```

- Search uses `websearch_to_tsquery` (supports `"exact phrase"`, `-exclude`, `or`) ranked by relevance
  when no sort is chosen; `SearchLike` adds substring matches (typo-tolerant prefix search for titles).
- Filter kinds: `String`, `Int`, `Bool`, `Date` (inclusive end), `Enum` (whitelist). Ops: `Eq`, `In`, `Gte`, `Lte`.
- Sort keys map to fixed SQL; always end with a unique column for stable pages.
- Pagination: offset + look-ahead row (`HasNext`), capped totals ("10,000+"), `MaxOffset` guard.
  For infinite feeds over huge tables, switch to keyset (`WHERE (created_at, id) < ($x, $y)`).

UI: `components.ListControls` (one GET form: search + filters + sort, htmx updates `#results` and the URL),
`FilterSelect`, `FilterDate`, `SortSelect`, `Pagination(page.Pager(base), "#results")`. Without JS the form
submits normally – same URL, same results.

Indexes to add with each Spec: `(owner_id, created_at DESC, id DESC)`, GIN on the tsvector, `gin_trgm_ops` on
`SearchLike` columns, and a btree for each filter column used on large tables.
For non-English content, use a language config (`FTSConfig: "german"`) in both the generated column and the Spec.
