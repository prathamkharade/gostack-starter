package listing

import (
	"net/url"
	"strings"
	"testing"
)

var spec = Spec{
	From:       "items",
	Select:     "id, title",
	SearchFTS:  "search",
	SearchLike: []string{"title"},
	Filters: []Filter{
		{Param: "status", Column: "status", Op: In, Kind: Enum, Values: []string{"draft", "published"}},
		{Param: "from", Column: "created_at", Op: Gte, Kind: Date},
		{Param: "to", Column: "created_at", Op: Lte, Kind: Date},
	},
	Sorts:       map[string]string{"newest": "created_at DESC, id DESC", "title": "title ASC, id ASC"},
	DefaultSort: "newest",
	CountLimit:  1000,
}

func TestParseDropsInvalidInput(t *testing.T) {
	v, _ := url.ParseQuery("q=+hello+&status=draft&status=evil';drop&from=2026-13-01&sort=hacked&page=-3&per_page=9999")
	p := spec.Parse(v)
	if p.Q != "hello" || p.Page != 1 || p.PerPage != 20 || p.Sort != "" {
		t.Fatalf("unexpected params %+v", p)
	}
	if len(p.Filters["status"]) != 1 || p.Filters["status"][0] != "draft" {
		t.Fatalf("enum filter not validated: %v", p.Filters)
	}
	if _, ok := p.Filters["from"]; ok {
		t.Fatal("invalid date must be dropped")
	}
}

func TestBuildIsParameterised(t *testing.T) {
	v, _ := url.ParseQuery("q=50%25_off&status=draft&status=published&from=2026-01-01&sort=title&page=2")
	p := spec.Parse(v)
	q, args, cq, cargs := spec.Build(p, "owner_id = $1", "user-1")

	if strings.Contains(q, "50") || strings.Contains(q, "draft") {
		t.Fatalf("user input leaked into SQL: %s", q)
	}
	want := "SELECT id, title FROM items WHERE (owner_id = $1) AND (search @@ websearch_to_tsquery('simple', $2) OR title ILIKE $3) AND status IN ($4, $5) AND created_at >= $6 ORDER BY title ASC, id ASC LIMIT $7 OFFSET $8"
	if q != want {
		t.Fatalf("query\n got: %s\nwant: %s", q, want)
	}
	if args[2] != `%50\%\_off%` {
		t.Fatalf("LIKE not escaped: %v", args[2])
	}
	if args[6] != 21 || args[7] != 20 {
		t.Fatalf("limit/offset wrong: %v %v", args[6], args[7])
	}
	if !strings.HasPrefix(cq, "SELECT count(*) FROM (SELECT 1 FROM items WHERE") || !strings.HasSuffix(cq, "LIMIT 1001) t") {
		t.Fatalf("count query: %s", cq)
	}
	if len(cargs) != 6 {
		t.Fatalf("count args should exclude limit/offset, got %d", len(cargs))
	}
}

func TestRelevanceSortWhenSearching(t *testing.T) {
	p := spec.Parse(url.Values{"q": {"go"}})
	q, _, _, _ := spec.Build(p, "")
	if !strings.Contains(q, "ORDER BY ts_rank(search, websearch_to_tsquery('simple', $1)) DESC, created_at DESC, id DESC") {
		t.Fatalf("expected relevance ordering: %s", q)
	}
	p = spec.Parse(url.Values{})
	q, _, _, _ = spec.Build(p, "")
	if !strings.Contains(q, "ORDER BY created_at DESC, id DESC LIMIT $1 OFFSET $2") || strings.Contains(q, "WHERE") {
		t.Fatalf("default query: %s", q)
	}
}

func TestMaxOffsetProtection(t *testing.T) {
	p := spec.Parse(url.Values{"page": {"100000"}})
	if (p.Page-1)*p.PerPage > 10_000 {
		t.Fatalf("offset not capped: page %d", p.Page)
	}
}

func TestPageAndURLs(t *testing.T) {
	p := spec.Parse(url.Values{"q": {"a b"}, "status": {"draft"}})
	rows := make([]int, 21)
	pg := NewPage(rows, p, 1500, 1000)
	if !pg.HasNext || len(pg.Items) != 20 || !pg.TotalCapped || pg.Total != 1000 {
		t.Fatalf("page: %+v", pg)
	}
	if got := p.WithPage(3).URL("/items"); got != "/items?page=3&q=a+b&status=draft" {
		t.Fatalf("url: %s", got)
	}
	pager := pg.Pager("/items")
	if !pager.HasNext || pager.NextURL != "/items?page=2&q=a+b&status=draft" || pager.HasPrev || pager.From != 1 || pager.To != 20 {
		t.Fatalf("pager: %+v", pager)
	}
	if p.WithSort("title").Page != 1 {
		t.Fatal("changing sort must reset page")
	}
	if !p.Has("status", "draft") || !p.Active() {
		t.Fatal("Has/Active")
	}
}
