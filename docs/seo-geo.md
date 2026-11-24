# SEO, GEO and AEO

- **SEO** – rank in search engines. **GEO** (generative engine optimisation) / **AEO** (answer engine
  optimisation) – be found and quoted correctly by AI assistants (ChatGPT search, Perplexity, Google AI
  Overviews, Claude). Both reward fast, crawlable, clearly structured, trustworthy pages.

## Built in
| Feature | Where |
|---|---|
| Title, description, canonical, Open Graph, Twitter card, `noindex` for private pages | `seo.Meta` → `layouts.Base` |
| JSON-LD: Organization, WebSite (+search), Article, FAQPage, BreadcrumbList | `internal/seo` helpers |
| `sitemap.xml` (static pages + published items) | `handlers_public.go` – add your public content there |
| `robots.txt` with AI crawler policy (`AI_CRAWLERS=allow/search-only/block`); dev/staging always disallow | `seo.Robots` |
| `llms.txt` – markdown map of your site for LLMs | `seo.LLMSTxt` |
| Semantic HTML, one `h1`, breadcrumbs, fast LCP, mobile-first | layouts/components |
| `security.txt`, web manifest, SVG favicon | `/.well-known/security.txt`, `/manifest.webmanifest` |

`search-only` (default) lets answer engines fetch pages to cite you (OAI-SearchBot, PerplexityBot,
Claude-SearchBot…) while opting out of model-training crawlers (GPTBot, ClaudeBot, Google-Extended, CCBot…).

## Content guidelines for GEO/AEO
1. Put a plain answer first: what it is, who it's for, price/availability – in 1-2 sentences near the top.
2. Use descriptive headings phrased like questions people ask; short paragraphs; lists and tables for facts.
3. Add FAQ sections (with `seo.FAQ` JSON-LD) on key pages.
4. Show authorship, dates (`datePublished`/`dateModified`), sources and contact details (trust signals).
5. Keep facts consistent across pages, `llms.txt` and structured data; update `dateModified` when content changes.
6. One canonical URL per piece of content; no content hidden behind JS.
7. Get listed where answer engines look: your own docs, reputable directories, review sites, communities.

## Per public page checklist
`seo.Meta{Title (≤ 60 chars), Description (≤ 155), Path, Image (1200×630), Type}` + relevant JSON-LD + sitemap entry + internal links.
