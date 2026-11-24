package config

import (
	"crypto/rand"
	"encoding/base64"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/joho/godotenv"
)

const examplePath = "../../.env.example"

// Keys in .env.example that are not read by the app itself.
var nonAppKeys = map[string]string{
	"DB_PORT":           "compose.yaml host port for dev Postgres",
	"POSTGRES_PASSWORD": "deploy/compose.prod.yaml",
	"DOMAIN":            "deploy/compose.prod.yaml (Caddy)",
	"APP_IMAGE":         "deploy/compose.prod.yaml",
}

// configKeys extracts every env key config.go reads, so this test never goes stale.
func configKeys(t *testing.T) map[string]bool {
	t.Helper()
	src, err := os.ReadFile("config.go")
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	for _, m := range regexp.MustCompile(`\b(?:str|num|ints|boolean|dur|list|secret)\("([A-Z][A-Z0-9_]*)"`).FindAllStringSubmatch(string(src), -1) {
		keys[m[1]] = true
	}
	if len(keys) < 20 {
		t.Fatalf("found only %d keys in config.go – has the helper naming changed?", len(keys))
	}
	return keys
}

func readExample(t *testing.T) map[string]string {
	t.Helper()
	env, err := godotenv.Read(examplePath)
	if err != nil {
		t.Fatalf(".env.example does not parse: %v", err)
	}
	return env
}

func TestEnvExampleDocumentsEveryConfigKey(t *testing.T) {
	env := readExample(t)
	var missing, unknown []string
	for k := range configKeys(t) {
		if _, ok := env[k]; !ok {
			missing = append(missing, k)
		}
	}
	known := configKeys(t)
	for k := range env {
		if !known[k] && nonAppKeys[k] == "" {
			unknown = append(unknown, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(unknown)
	if len(missing) > 0 {
		t.Errorf("config reads these keys but .env.example does not document them: %v", missing)
	}
	if len(unknown) > 0 {
		t.Errorf(".env.example has keys nothing reads (typo or stale?): %v", unknown)
	}
}

func TestEnvExampleHasNoLeakedComments(t *testing.T) {
	for k, v := range readExample(t) {
		if strings.HasPrefix(strings.TrimSpace(v), "#") {
			t.Errorf("%s parses as %q: put the comment on its own line above the key", k, v)
		}
	}
}

func TestEnvExampleLoadsWithFreshSecrets(t *testing.T) {
	for k, v := range readExample(t) {
		t.Setenv(k, v)
	}
	rnd := func() string {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		return base64.StdEncoding.EncodeToString(b)
	}
	// what `make setup` fills in
	t.Setenv("JWT_SECRET", rnd())
	t.Setenv("BLIND_INDEX_KEY", rnd())
	t.Setenv("ENCRYPTION_KEYS", "v1:"+rnd())
	if os.Getenv("DATABASE_URL") == "" {
		t.Setenv("DATABASE_URL", "postgres://app:app@localhost:5433/app?sslmode=disable")
	}
	c, err := Load()
	if err != nil {
		t.Fatalf(".env.example + fresh secrets must load: %v", err)
	}
	if !c.IsDev() {
		t.Errorf(".env.example should default to development, got %q", c.Env)
	}
}
