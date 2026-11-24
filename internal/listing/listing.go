// Package listing turns URL query parameters into safe, parameterised SQL for
// list pages: full-text search, fuzzy search, whitelisted filters, whitelisted
// sorting and pagination. It never interpolates user input into SQL — only
// developer-defined column names from the Spec, plus $n placeholders.
//
// Usage (see internal/items/repo.go for a complete example):
//
//	var spec = listing.Spec{From: "items", Select: "id, title", SearchFTS: "search", ...}
//	p := spec.Parse(queryValues)
//	q, args, cq, cargs := spec.Build(p, "owner_id = $1", userID)
package listing

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Kind int

const (
	String Kind = iota
	Int
	Bool
	Date // YYYY-MM-DD
	Enum // value must be in Filter.Values
)

type Op string

const (
	Eq  Op = "="
	In  Op = "IN" // multiple values: ?status=a&status=b
	Gte Op = ">="
	Lte Op = "<="
)

// Filter maps a URL parameter to a trusted SQL column/expression.
type Filter struct {
	Param  string
	Column string
	Op     Op
	Kind   Kind
	Values []string // allowed values for Enum (also used to render filter UI)
	Label  string   // UI label
}

// Spec describes one listable resource. All strings are trusted developer input.
type Spec struct {
	From           string            // FROM clause, e.g. "items" or "items i JOIN users u ON ..."
	Select         string            // column list
	SearchFTS      string            // tsvector column (GIN index), optional
	FTSConfig      string            // text search config, default "simple"
	SearchLike     []string          // columns searched with ILIKE (pg_trgm GIN index), optional
	Filters        []Filter          // allowed filters
	Sorts          map[string]string // sort key -> ORDER BY expression (must end with a unique column, e.g. ", id DESC")
	DefaultSort    string            // key in Sorts
	DefaultPerPage int               // default 20
	MaxPerPage     int               // default 100
	CountLimit     int               // count at most N rows (0 = exact count). Cheap "10,000+" totals.
	MaxOffset      int               // reject deeper offsets (default 10_000) – protects the DB
}

// Params is the validated, normalised request.
type Params struct {
	Q       string
	Filters map[string][]string
	Sort    string
	Page    int
	PerPage int
}

const relevance = "relevance"

// Parse validates url.Values against the spec. Invalid values are dropped, never errors:
// a list page should always render.
func (s Spec) Parse(v url.Values) Params {
	p := Params{
		Q:       strings.TrimSpace(v.Get("q")),
		Filters: map[string][]string{},
		Sort:    v.Get("sort"),
		Page:    atoi(v.Get("page"), 1),
		PerPage: atoi(v.Get("per_page"), s.perPage()),
	}
	if len(p.Q) > 200 {
		p.Q = p.Q[:200]
	}
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PerPage < 1 || p.PerPage > s.maxPerPage() {
		p.PerPage = s.perPage()
	}
	if (p.Page-1)*p.PerPage > s.maxOffset() {
		p.Page = s.maxOffset()/p.PerPage + 1
	}
	if _, ok := s.Sorts[p.Sort]; !ok && !(p.Sort == relevance && p.Q != "" && s.SearchFTS != "") {
		p.Sort = ""
	}
	for _, f := range s.Filters {
		for _, raw := range v[f.Param] {
			if val, ok := f.valid(raw); ok {
				p.Filters[f.Param] = append(p.Filters[f.Param], val)
			}
			if f.Op != In {
				break
			}
		}
	}
	return p
}

// Build returns the page query and the count query. scope is an optional SQL
// condition using $1..$len(scopeArgs), e.g. "owner_id = $1".
func (s Spec) Build(p Params, scope string, scopeArgs ...any) (query string, args []any, countQuery string, countArgs []any) {
	args = append(args, scopeArgs...)
	var where []string
	if scope != "" {
		where = append(where, "("+scope+")")
	}
	next := func(v any) string { args = append(args, v); return "$" + strconv.Itoa(len(args)) }

	var tsq string
	if p.Q != "" {
		var ors []string
		if s.SearchFTS != "" {
			tsq = fmt.Sprintf("websearch_to_tsquery('%s', %s)", s.ftsConfig(), next(p.Q))
			ors = append(ors, s.SearchFTS+" @@ "+tsq)
		}
		if len(s.SearchLike) > 0 {
			ph := next("%" + EscapeLike(p.Q) + "%")
			for _, col := range s.SearchLike {
				ors = append(ors, col+" ILIKE "+ph)
			}
		}
		if len(ors) > 0 {
			where = append(where, "("+strings.Join(ors, " OR ")+")")
		}
	}

	for _, f := range s.Filters {
		vals := p.Filters[f.Param]
		if len(vals) == 0 {
			continue
		}
		if f.Op == In {
			phs := make([]string, len(vals))
			for i, v := range vals {
				phs[i] = next(f.cast(v))
			}
			where = append(where, f.Column+" IN ("+strings.Join(phs, ", ")+")")
			continue
		}
		op := f.Op
		if op == "" {
			op = Eq
		}
		where = append(where, f.Column+" "+string(op)+" "+next(f.cast(vals[0])))
	}

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}

	// count query shares the filter args (but not limit/offset)
	countArgs = append([]any(nil), args...)
	if s.CountLimit > 0 {
		countQuery = fmt.Sprintf("SELECT count(*) FROM (SELECT 1 FROM %s%s LIMIT %d) t", s.From, whereSQL, s.CountLimit+1)
	} else {
		countQuery = fmt.Sprintf("SELECT count(*) FROM %s%s", s.From, whereSQL)
	}

	order := s.Sorts[p.Sort]
	switch {
	case p.Sort == relevance && tsq != "":
		order = "ts_rank(" + s.SearchFTS + ", " + tsq + ") DESC"
		if def, ok := s.Sorts[s.DefaultSort]; ok {
			order += ", " + def
		}
	case order == "" && p.Q != "" && tsq != "":
		order = "ts_rank(" + s.SearchFTS + ", " + tsq + ") DESC"
		if def, ok := s.Sorts[s.DefaultSort]; ok {
			order += ", " + def
		}
	case order == "":
		order = s.Sorts[s.DefaultSort]
	}
	orderSQL := ""
	if order != "" {
		orderSQL = " ORDER BY " + order
	}

	limit := next(p.PerPage + 1) // one extra row tells us if there is a next page
	offset := next((p.Page - 1) * p.PerPage)
	query = fmt.Sprintf("SELECT %s FROM %s%s%s LIMIT %s OFFSET %s", s.Select, s.From, whereSQL, orderSQL, limit, offset)
	return query, args, countQuery, countArgs
}

// EscapeLike escapes %, _ and \ for use inside a LIKE pattern.
func EscapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// --- Page result ---------------------------------------------------------------

// Page is what handlers pass to templates.
type Page[T any] struct {
	Items       []T
	Params      Params
	Total       int  // -1 when not counted
	TotalCapped bool // true when Total == CountLimit and more exist
	HasNext     bool
}

// NewPage trims the extra look-ahead row and records whether more exist.
func NewPage[T any](rows []T, p Params, total int, countLimit int) Page[T] {
	pg := Page[T]{Params: p, Total: total}
	if len(rows) > p.PerPage {
		rows = rows[:p.PerPage]
		pg.HasNext = true
	}
	if countLimit > 0 && total > countLimit {
		pg.Total, pg.TotalCapped = countLimit, true
	}
	pg.Items = rows
	return pg
}

func (p Page[T]) HasPrev() bool { return p.Params.Page > 1 }
func (p Page[T]) From() int {
	if len(p.Items) == 0 {
		return 0
	}
	return (p.Params.Page-1)*p.Params.PerPage + 1
}
func (p Page[T]) To() int { return (p.Params.Page-1)*p.Params.PerPage + len(p.Items) }
func (p Page[T]) TotalPages() int {
	if p.Total <= 0 {
		return 0
	}
	return (p.Total + p.Params.PerPage - 1) / p.Params.PerPage
}

// Pager is a non-generic summary for the pagination component.
type Pager struct {
	Page, From, To, Total, TotalPages int
	Capped, HasPrev, HasNext          bool
	PrevURL, NextURL                  string
}

// Pager builds links that keep the current search/filter/sort.
func (p Page[T]) Pager(base string) Pager {
	pg := Pager{Page: p.Params.Page, From: p.From(), To: p.To(), Total: p.Total, TotalPages: p.TotalPages(),
		Capped: p.TotalCapped, HasPrev: p.HasPrev(), HasNext: p.HasNext}
	if pg.HasPrev {
		pg.PrevURL = p.Params.WithPage(p.Params.Page - 1).URL(base)
	}
	if pg.HasNext {
		pg.NextURL = p.Params.WithPage(p.Params.Page + 1).URL(base)
	}
	return pg
}

// --- URL helpers (keep filters when changing page/sort) -----------------------

// Values re-encodes params for links. Default values are omitted for clean URLs.
func (p Params) Values() url.Values {
	v := url.Values{}
	if p.Q != "" {
		v.Set("q", p.Q)
	}
	if p.Sort != "" {
		v.Set("sort", p.Sort)
	}
	if p.Page > 1 {
		v.Set("page", strconv.Itoa(p.Page))
	}
	for k, vals := range p.Filters {
		for _, val := range vals {
			v.Add(k, val)
		}
	}
	return v
}

func (p Params) URL(base string) string {
	if enc := p.Values().Encode(); enc != "" {
		return base + "?" + enc
	}
	return base
}

func (p Params) WithPage(n int) Params    { p.Page = n; return p }
func (p Params) WithSort(s string) Params { p.Sort = s; p.Page = 1; return p }

// Has reports whether a filter currently has the given value (for checkboxes/selects).
func (p Params) Has(param, value string) bool {
	for _, v := range p.Filters[param] {
		if v == value {
			return true
		}
	}
	return false
}

func (p Params) First(param string) string {
	if v := p.Filters[param]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// Active reports whether any search/filter is applied (for "clear filters" UI).
func (p Params) Active() bool { return p.Q != "" || len(p.Filters) > 0 }

// --- internals -------------------------------------------------------------------

func (f Filter) valid(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 100 {
		return "", false
	}
	switch f.Kind {
	case Int:
		_, err := strconv.ParseInt(raw, 10, 64)
		return raw, err == nil
	case Bool:
		b, err := strconv.ParseBool(raw)
		return strconv.FormatBool(b), err == nil
	case Date:
		_, err := time.Parse(time.DateOnly, raw)
		return raw, err == nil
	case Enum:
		for _, v := range f.Values {
			if v == raw {
				return raw, true
			}
		}
		return "", false
	default:
		return raw, true
	}
}

func (f Filter) cast(v string) any {
	switch f.Kind {
	case Int:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	case Bool:
		b, _ := strconv.ParseBool(v)
		return b
	case Date:
		t, _ := time.Parse(time.DateOnly, v)
		if f.Op == Lte { // inclusive end date
			return t.Add(24*time.Hour - time.Nanosecond)
		}
		return t
	default:
		return v
	}
}

func (s Spec) perPage() int {
	if s.DefaultPerPage > 0 {
		return s.DefaultPerPage
	}
	return 20
}

func (s Spec) maxPerPage() int {
	if s.MaxPerPage > 0 {
		return s.MaxPerPage
	}
	return 100
}

func (s Spec) maxOffset() int {
	if s.MaxOffset > 0 {
		return s.MaxOffset
	}
	return 10_000
}

func (s Spec) ftsConfig() string {
	if s.FTSConfig != "" {
		return s.FTSConfig
	}
	return "simple"
}

func atoi(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}
