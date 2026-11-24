//go:build integration

// Integration tests: real Postgres, real HTTP stack (app.Test, no network).
// Run: make test-integration   (needs TEST_DATABASE_URL, set automatically)
package web

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"app/internal/auth"
	"app/internal/config"
	"app/internal/database"
	"app/internal/items"
	"app/internal/jobs"
	"app/internal/media"
	"app/internal/notify"
	"app/internal/privacy"
	"app/internal/security"
	"app/internal/storage"
)

type env struct {
	t    *testing.T
	srv  *Server
	keys *security.Keyring
	cfg  *config.Config
}

func key() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func setup(t *testing.T) *env {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool, err := database.Open(ctx, dsn, 5)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := database.Migrate(ctx, pool, log); err != nil {
		t.Fatal(err)
	}
	jwtKey, _ := base64.StdEncoding.DecodeString(key())
	bidx, _ := base64.StdEncoding.DecodeString(key())
	cfg := &config.Config{Env: "test", AppName: "Test", BaseURL: "http://localhost:8080", JWTSecret: jwtKey, BlindIndexKey: bidx,
		EncryptionKeys: "v1:" + key(), AccessTTL: 15 * time.Minute, RefreshTTL: time.Hour, MediaMaxUploadMB: 5,
		DefaultLocale: "en", AICrawlers: "search-only"}
	keys, err := security.NewKeyring(cfg.EncryptionKeys)
	if err != nil {
		t.Fatal(err)
	}
	q := jobs.New(pool, keys, log)
	store, _ := storage.NewLocal(t.TempDir())
	ms := media.NewService(pool, store, media.NewProcessor(media.Options{}), q, keys, log, 5, false, "")
	a := auth.New(pool, keys, cfg, q, log)
	srv := New(Deps{Cfg: cfg, Log: log, Pool: pool, Auth: a, Media: ms, Privacy: privacy.New(pool, a, q, "Test", log), Items: items.NewRepo(pool)})
	return &env{t: t, srv: srv, keys: keys, cfg: cfg}
}

// do sends a same-origin browser-like request.
func (e *env) do(method, path string, form url.Values, cookies []*http.Cookie) *http.Response {
	e.t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	res, err := e.srv.App.Test(req, fiber.TestConfig{Timeout: 10 * time.Second})
	if err != nil {
		e.t.Fatal(err)
	}
	return res
}

func readBody(res *http.Response) string {
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	return string(b)
}

// lastEmailCode decrypts the newest queued email and extracts the 6-digit code.
func (e *env) lastEmailCode() string {
	var payload []byte
	err := e.srv.pool.QueryRow(context.Background(),
		"SELECT payload FROM jobs WHERE kind = 'email.send' ORDER BY id DESC LIMIT 1").Scan(&payload)
	if err != nil {
		e.t.Fatal(err)
	}
	plain, err := e.keys.Decrypt(payload, "jobs.payload")
	if err != nil {
		e.t.Fatal(err)
	}
	var m notify.Email
	_ = json.Unmarshal([]byte(plain), &m)
	code := regexp.MustCompile(`\b\d{6}\b`).FindString(m.Subject)
	if code == "" {
		e.t.Fatalf("no code in %q", m.Subject)
	}
	return code
}

func TestSecurityHeadersAndSEO(t *testing.T) {
	e := setup(t)
	res := e.do("GET", "/", nil, nil)
	if res.StatusCode != 200 {
		t.Fatalf("home: %d", res.StatusCode)
	}
	csp := res.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self'") || strings.Contains(csp, "unsafe-eval") {
		t.Fatalf("weak CSP: %s", csp)
	}
	if res.Header.Get("X-Content-Type-Options") != "nosniff" || res.Header.Get("X-Frame-Options") != "DENY" {
		t.Fatal("missing security headers")
	}
	html := readBody(res)
	if !strings.Contains(html, `<link rel="canonical"`) || !strings.Contains(html, "application/ld+json") {
		t.Fatal("home must have canonical + JSON-LD")
	}
	for _, p := range []string{"/robots.txt", "/sitemap.xml", "/llms.txt", "/.well-known/security.txt"} {
		if r := e.do("GET", p, nil, nil); r.StatusCode != 200 {
			t.Errorf("%s: %d", p, r.StatusCode)
		}
	}
}

func TestCrossOriginPostIsBlocked(t *testing.T) {
	e := setup(t)
	req := httptest.NewRequest("POST", "/login", strings.NewReader("email=a@b.co"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("Origin", "https://evil.example")
	res, _ := e.srv.App.Test(req)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-site POST must be rejected, got %d", res.StatusCode)
	}
}

func TestProtectedRouteRedirects(t *testing.T) {
	e := setup(t)
	res := e.do("GET", "/items", nil, nil)
	if res.StatusCode != http.StatusSeeOther || !strings.HasPrefix(res.Header.Get("Location"), "/login?next=") {
		t.Fatalf("got %d %s", res.StatusCode, res.Header.Get("Location"))
	}
}

func TestSignupItemsExportAndDelete(t *testing.T) {
	e := setup(t)
	// lower-case: the app normalises emails, and the assertions below look for this exact string
	email := strings.ToLower("it-" + security.RandomToken(6) + "@example.com")

	// 1. request a code (sign-up and sign-in are the same flow)
	if res := e.do("POST", "/login", url.Values{"email": {email}}, nil); res.StatusCode != 200 {
		t.Fatalf("login start: %d", res.StatusCode)
	}
	// wrong code is rejected
	if res := e.do("POST", "/login/verify", url.Values{"email": {email}, "code": {"000000"}}, nil); res.StatusCode != 422 {
		t.Fatalf("wrong code: %d", res.StatusCode)
	}
	// 2. verify
	res := e.do("POST", "/login/verify", url.Values{"email": {email}, "code": {e.lastEmailCode()}}, nil)
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("verify: %d %s", res.StatusCode, readBody(res))
	}
	cookies := res.Cookies()
	if len(cookies) < 2 {
		t.Fatal("expected auth cookies")
	}
	for _, c := range cookies {
		if (c.Name == "at" || c.Name == "rt") && !c.HttpOnly {
			t.Fatalf("cookie %s must be HttpOnly", c.Name)
		}
	}

	// 3. email is encrypted at rest
	var enc []byte
	_ = e.srv.pool.QueryRow(context.Background(), "SELECT email_enc FROM users ORDER BY created_at DESC LIMIT 1").Scan(&enc)
	if strings.Contains(string(enc), email) {
		t.Fatal("email stored in plain text")
	}

	// 4. account page shows decrypted email
	acc := e.do("GET", "/account", nil, cookies)
	if body := readBody(acc); !strings.Contains(body, email) {
		t.Fatalf("account page should show email: status %d, location %q, body %.300s",
			acc.StatusCode, acc.Header.Get("Location"), body)
	}

	// 5. create + search an item
	if res := e.do("POST", "/items", url.Values{"title": {"Integration widget"}, "status": {"published"}}, cookies); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("create item: %d", res.StatusCode)
	}
	if body := readBody(e.do("GET", "/items?q=widget", nil, cookies)); !strings.Contains(body, "Integration widget") {
		t.Fatal("search should find the item")
	}
	if body := readBody(e.do("GET", "/items?q=nomatchxyz", nil, cookies)); strings.Contains(body, "Integration widget") {
		t.Fatal("search should filter")
	}
	// validation
	if res := e.do("POST", "/items", url.Values{"title": {""}, "status": {"hacked"}}, cookies); res.StatusCode != 422 {
		t.Fatalf("invalid item must be 422, got %d", res.StatusCode)
	}

	// 6. export contains the account and the item
	exp := readBody(e.do("GET", "/account/export", nil, cookies))
	if !strings.Contains(exp, email) || !strings.Contains(exp, "Integration widget") {
		t.Fatalf("export incomplete: %s", exp[:min(300, len(exp))])
	}

	// 7. delete account: everything owned is gone
	if res := e.do("POST", "/account/delete", url.Values{"confirm": {"DELETE"}}, cookies); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete: %d", res.StatusCode)
	}
	var n int
	_ = e.srv.pool.QueryRow(context.Background(), "SELECT count(*) FROM items WHERE title = 'Integration widget'").Scan(&n)
	if n != 0 {
		t.Fatal("items must be deleted with the account")
	}
	if res := e.do("GET", "/account", nil, cookies); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("old session must not work after deletion, got %d", res.StatusCode)
	}
}
