// Package seo produces everything search engines and AI answer engines read:
// <head> metadata (title, description, canonical, Open Graph, Twitter), JSON-LD
// structured data, sitemap.xml, robots.txt (with AI-crawler policy) and llms.txt.
//
// SEO = rank in classic search. GEO/AEO = be quoted correctly by AI answers
// (ChatGPT, Perplexity, Google AI Overviews, Claude). Both reward the same
// things: fast server-rendered HTML, clear headings, structured data, an
// explicit crawler policy and a concise machine-readable site summary.
package seo

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

// Meta is set by each handler; the base layout renders it into <head>.
type Meta struct {
	Title       string
	Description string
	Path        string // canonical path, e.g. "/pricing"
	Image       string // absolute or root-relative OG image
	Type        string // website | article | product
	NoIndex     bool   // private pages (account, dashboards, forms)
	Published   time.Time
	Modified    time.Time
	JSONLD      []any // schema.org objects
}

// FullTitle returns "Page – App" (or just App on the home page).
func (m Meta) FullTitle(app string) string {
	if m.Title == "" || m.Title == app {
		return app
	}
	return m.Title + " – " + app
}

func Abs(base, p string) string {
	if p == "" || strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		return p
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(p, "/")
}

// JSONLDScript renders a <script type="application/ld+json"> block. JSON-LD is
// data (not executed), so it is allowed by a strict CSP without a nonce.
func JSONLDScript(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	// prevent "</script>" breaking out of the block
	s := strings.NewReplacer("<", `<`, ">", `>`, "&", `&`).Replace(string(b))
	return `<script type="application/ld+json">` + s + `</script>`
}

// --- schema.org helpers -------------------------------------------------------------

func Organization(name, url, logo string) map[string]any {
	return map[string]any{"@context": "https://schema.org", "@type": "Organization", "name": name, "url": url, "logo": logo}
}

func WebSite(name, url string) map[string]any {
	return map[string]any{"@context": "https://schema.org", "@type": "WebSite", "name": name, "url": url,
		"potentialAction": map[string]any{"@type": "SearchAction", "target": url + "/search?q={query}", "query-input": "required name=query"}}
}

func Article(headline, description, url, image string, published, modified time.Time, author string) map[string]any {
	a := map[string]any{"@context": "https://schema.org", "@type": "Article", "headline": headline,
		"description": description, "mainEntityOfPage": url, "datePublished": published.Format(time.RFC3339),
		"dateModified": modified.Format(time.RFC3339), "author": map[string]any{"@type": "Organization", "name": author}}
	if image != "" {
		a["image"] = image
	}
	return a
}

// FAQ is one of the most-quoted formats by answer engines.
func FAQ(qa map[string]string) map[string]any {
	var items []map[string]any
	for q, a := range qa {
		items = append(items, map[string]any{"@type": "Question", "name": q,
			"acceptedAnswer": map[string]any{"@type": "Answer", "text": a}})
	}
	return map[string]any{"@context": "https://schema.org", "@type": "FAQPage", "mainEntity": items}
}

func Breadcrumbs(base string, crumbs [][2]string) map[string]any {
	var items []map[string]any
	for i, c := range crumbs {
		items = append(items, map[string]any{"@type": "ListItem", "position": i + 1, "name": c[0], "item": Abs(base, c[1])})
	}
	return map[string]any{"@context": "https://schema.org", "@type": "BreadcrumbList", "itemListElement": items}
}

// --- sitemap ---------------------------------------------------------------------------

type URL struct {
	Loc     string    `xml:"loc"`
	LastMod string    `xml:"lastmod,omitempty"`
	Changed time.Time `xml:"-"`
}

// Sitemap renders a sitemap.xml (max 50k URLs; split into an index beyond that).
func Sitemap(base string, urls []URL) ([]byte, error) {
	type urlset struct {
		XMLName xml.Name `xml:"urlset"`
		NS      string   `xml:"xmlns,attr"`
		URLs    []URL    `xml:"url"`
	}
	for i := range urls {
		urls[i].Loc = Abs(base, urls[i].Loc)
		if !urls[i].Changed.IsZero() {
			urls[i].LastMod = urls[i].Changed.UTC().Format("2006-01-02")
		}
	}
	b, err := xml.MarshalIndent(urlset{NS: "http://www.sitemaps.org/schemas/sitemap/0.9", URLs: urls}, "", "  ")
	return append([]byte(xml.Header), b...), err
}

// --- robots.txt ------------------------------------------------------------------------

// AI crawlers, split by purpose. "search" bots fetch pages to cite them in
// answers (good for GEO); "training" bots collect data to train models.
var (
	AISearchBots   = []string{"OAI-SearchBot", "ChatGPT-User", "PerplexityBot", "Perplexity-User", "Claude-SearchBot", "Claude-User"}
	AITrainingBots = []string{"GPTBot", "ClaudeBot", "Google-Extended", "Applebot-Extended", "CCBot", "Bytespider", "meta-externalagent"}
)

// Robots builds robots.txt. policy: "allow" (all AI bots), "search-only"
// (allow answer engines, block training) or "block" (block all AI bots).
func Robots(base, policy string, disallow []string) string {
	var b strings.Builder
	block := func(bots []string) {
		for _, bot := range bots {
			fmt.Fprintf(&b, "User-agent: %s\nDisallow: /\n\n", bot)
		}
	}
	switch policy {
	case "block":
		block(AISearchBots)
		block(AITrainingBots)
	case "search-only":
		block(AITrainingBots)
	}
	b.WriteString("User-agent: *\n")
	for _, d := range disallow {
		b.WriteString("Disallow: " + d + "\n")
	}
	b.WriteString("Allow: /\n\nSitemap: " + base + "/sitemap.xml\n")
	return b.String()
}

// LLMSTxt renders /llms.txt (llmstxt.org proposal): a short markdown map of the
// site for language models. Keep it factual and up to date.
func LLMSTxt(app, description, base string, sections map[string][][2]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n> %s\n\n", app, description)
	for title, links := range sections {
		fmt.Fprintf(&b, "## %s\n\n", title)
		for _, l := range links {
			fmt.Fprintf(&b, "- [%s](%s)\n", l[0], Abs(base, l[1]))
		}
		b.WriteString("\n")
	}
	return b.String()
}
