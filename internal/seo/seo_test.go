package seo

import (
	"strings"
	"testing"
	"time"
)

func TestJSONLDEscapesScriptBreakout(t *testing.T) {
	out := string(JSONLDScript(map[string]string{"name": "</script><script>alert(1)</script>"}))
	if strings.Count(out, "</script>") != 1 {
		t.Fatalf("script breakout possible: %s", out)
	}
}

func TestRobotsPolicies(t *testing.T) {
	r := Robots("https://x.com", "search-only", []string{"/account"})
	if !strings.Contains(r, "User-agent: GPTBot\nDisallow: /") || strings.Contains(r, "User-agent: OAI-SearchBot") {
		t.Fatalf("search-only policy wrong:\n%s", r)
	}
	if !strings.Contains(r, "Sitemap: https://x.com/sitemap.xml") || !strings.Contains(r, "Disallow: /account") {
		t.Fatal("missing sitemap/disallow")
	}
	if !strings.Contains(Robots("https://x.com", "block", nil), "User-agent: PerplexityBot") {
		t.Fatal("block policy must block search bots too")
	}
}

func TestSitemap(t *testing.T) {
	b, err := Sitemap("https://x.com", []URL{{Loc: "/"}, {Loc: "/i/1", Changed: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)}})
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "<loc>https://x.com/i/1</loc>") || !strings.Contains(s, "<lastmod>2026-01-02</lastmod>") {
		t.Fatalf("sitemap: %s", s)
	}
}

func TestMeta(t *testing.T) {
	if (Meta{Title: "Pricing"}).FullTitle("App") != "Pricing – App" || (Meta{}).FullTitle("App") != "App" {
		t.Fatal("FullTitle")
	}
	if Abs("https://x.com/", "/a") != "https://x.com/a" {
		t.Fatal("Abs")
	}
}
