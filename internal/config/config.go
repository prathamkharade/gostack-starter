// Package config loads every runtime setting from environment variables (and an
// optional .env file in development). It is the ONLY place that reads os.Getenv.
// To add a setting: add a field, read it in Load, document it in .env.example.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Env      string // development | production | test
	AppName  string
	BaseURL  string // public URL, no trailing slash, e.g. https://example.com
	Addr     string // listen address, e.g. :8080
	LogLevel string // debug | info | warn | error

	// Organisation / legal (shown on privacy pages, security.txt, emails)
	OrgName         string
	OrgEmail        string // privacy / data-protection contact
	SecurityContact string // mailto: or https: URL for security.txt
	DefaultLocale   string

	// Database
	DatabaseURL string
	DBMaxConns  int32

	// Security
	JWTSecret      []byte
	EncryptionKeys string // "v1:base64key[,v0:base64key]" – first key encrypts, all decrypt
	BlindIndexKey  []byte
	CookieSecure   bool
	TrustedProxies []string
	AccessTTL      time.Duration
	RefreshTTL     time.Duration

	// Email (any SMTP provider: SES, Brevo, Resend, Postmark, Mailgun, Mailpit in dev)
	MailFrom string
	SMTPHost string
	SMTPPort int
	SMTPUser string
	SMTPPass string
	SMTPTLS  string // starttls | tls | none

	// SMS (log | twilio | vonage)
	SMSProvider  string
	SMSFrom      string
	TwilioSID    string
	TwilioToken  string
	VonageKey    string
	VonageSecret string

	// Storage (local | s3). S3 works with Cloudflare R2, Backblaze B2, Hetzner, MinIO, AWS.
	StorageDriver   string
	StorageLocalDir string
	S3Endpoint      string
	S3Region        string
	S3Bucket        string
	S3AccessKey     string
	S3SecretKey     string
	S3PublicURL     string // optional CDN/public base URL for public objects
	S3PathStyle     bool

	// Media processing
	MediaMaxUploadMB    int
	MediaImageFormat    string // webp | avif
	MediaImageQuality   int
	MediaImageWidths    []int
	MediaVideoCodec     string // h264 | av1
	MediaVideoMaxHeight int
	MediaKeepOriginal   bool

	// Privacy / retention (days)
	RetentionAuditDays int
	RetentionJobsDays  int

	// SEO / GEO
	SiteDescription string
	AICrawlers      string // allow | search-only | block

	// Workers
	WorkerConcurrency int
}

func (c *Config) IsProd() bool { return c.Env == "production" }
func (c *Config) IsDev() bool  { return c.Env == "development" }

// Load reads configuration. It never logs secret values.
func Load() (*Config, error) {
	_ = godotenv.Load() // .env is optional; real env vars always win

	c := &Config{
		Env:      str("APP_ENV", "development"),
		AppName:  str("APP_NAME", "MyApp"),
		BaseURL:  strings.TrimRight(str("BASE_URL", "http://localhost:8080"), "/"),
		Addr:     str("ADDR", ":8080"),
		LogLevel: str("LOG_LEVEL", "info"),

		OrgName:         str("ORG_NAME", "MyApp Ltd"),
		OrgEmail:        str("ORG_EMAIL", "privacy@example.com"),
		SecurityContact: str("SECURITY_CONTACT", "mailto:security@example.com"),
		DefaultLocale:   str("DEFAULT_LOCALE", "en"),

		DatabaseURL: str("DATABASE_URL", ""),
		DBMaxConns:  int32(num("DB_MAX_CONNS", 10)),

		EncryptionKeys: str("ENCRYPTION_KEYS", ""),
		CookieSecure:   boolean("COOKIE_SECURE", false),
		TrustedProxies: list("TRUSTED_PROXIES", nil),
		AccessTTL:      dur("ACCESS_TTL", 15*time.Minute),
		RefreshTTL:     dur("REFRESH_TTL", 30*24*time.Hour),

		MailFrom: str("MAIL_FROM", "MyApp <no-reply@example.com>"),
		SMTPHost: str("SMTP_HOST", "localhost"),
		SMTPPort: num("SMTP_PORT", 1025),
		SMTPUser: str("SMTP_USER", ""),
		SMTPPass: str("SMTP_PASS", ""),
		SMTPTLS:  str("SMTP_TLS", "none"),

		SMSProvider:  str("SMS_PROVIDER", "log"),
		SMSFrom:      str("SMS_FROM", ""),
		TwilioSID:    str("TWILIO_ACCOUNT_SID", ""),
		TwilioToken:  str("TWILIO_AUTH_TOKEN", ""),
		VonageKey:    str("VONAGE_API_KEY", ""),
		VonageSecret: str("VONAGE_API_SECRET", ""),

		StorageDriver:   str("STORAGE_DRIVER", "local"),
		StorageLocalDir: str("STORAGE_LOCAL_DIR", "./data/media"),
		S3Endpoint:      str("S3_ENDPOINT", ""),
		S3Region:        str("S3_REGION", "auto"),
		S3Bucket:        str("S3_BUCKET", ""),
		S3AccessKey:     str("S3_ACCESS_KEY", ""),
		S3SecretKey:     str("S3_SECRET_KEY", ""),
		S3PublicURL:     strings.TrimRight(str("S3_PUBLIC_URL", ""), "/"),
		S3PathStyle:     boolean("S3_PATH_STYLE", true),

		MediaMaxUploadMB:    num("MEDIA_MAX_UPLOAD_MB", 64),
		MediaImageFormat:    str("MEDIA_IMAGE_FORMAT", "webp"),
		MediaImageQuality:   num("MEDIA_IMAGE_QUALITY", 75),
		MediaImageWidths:    ints("MEDIA_IMAGE_WIDTHS", []int{320, 640, 1280, 1920}),
		MediaVideoCodec:     str("MEDIA_VIDEO_CODEC", "h264"),
		MediaVideoMaxHeight: num("MEDIA_VIDEO_MAX_HEIGHT", 720),
		MediaKeepOriginal:   boolean("MEDIA_KEEP_ORIGINAL", false),

		RetentionAuditDays: num("RETENTION_AUDIT_DAYS", 365),
		RetentionJobsDays:  num("RETENTION_JOBS_DAYS", 7),

		SiteDescription: str("SITE_DESCRIPTION", "A fast, privacy-friendly web application."),
		AICrawlers:      str("AI_CRAWLERS", "search-only"),

		WorkerConcurrency: num("WORKER_CONCURRENCY", 2),
	}

	var err error
	if c.JWTSecret, err = secret("JWT_SECRET"); err != nil {
		return nil, err
	}
	if c.BlindIndexKey, err = secret("BLIND_INDEX_KEY"); err != nil {
		return nil, err
	}
	return c, c.validate()
}

func (c *Config) validate() error {
	var errs []error
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if c.EncryptionKeys == "" {
		errs = append(errs, errors.New("ENCRYPTION_KEYS is required (run: go run ./cmd/app keygen)"))
	}
	if c.IsProd() {
		if !c.CookieSecure {
			errs = append(errs, errors.New("COOKIE_SECURE must be true in production"))
		}
		if !strings.HasPrefix(c.BaseURL, "https://") {
			errs = append(errs, errors.New("BASE_URL must be https in production"))
		}
	}
	if c.StorageDriver == "s3" && (c.S3Bucket == "" || c.S3Endpoint == "" || c.S3AccessKey == "") {
		errs = append(errs, errors.New("S3_BUCKET, S3_ENDPOINT and S3_ACCESS_KEY are required for STORAGE_DRIVER=s3"))
	}
	return errors.Join(errs...)
}

// --- helpers -----------------------------------------------------------------

func str(k, def string) string {
	v, ok := os.LookupEnv(k)
	v = strings.TrimSpace(v)
	// `KEY= # comment` in a .env file can reach us as "#" or "# comment": that means "not set".
	if !ok || v == "" || strings.HasPrefix(v, "#") {
		return def
	}
	return v
}

func num(k string, def int) int {
	if v, err := strconv.Atoi(str(k, "")); err == nil {
		return v
	}
	return def
}

func boolean(k string, def bool) bool {
	if v, err := strconv.ParseBool(str(k, "")); err == nil {
		return v
	}
	return def
}

func dur(k string, def time.Duration) time.Duration {
	if v, err := time.ParseDuration(str(k, "")); err == nil {
		return v
	}
	return def
}

func list(k string, def []string) []string {
	v := str(k, "")
	if v == "" {
		return def
	}
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func ints(k string, def []int) []int {
	var out []int
	for _, p := range list(k, nil) {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}

// secret reads a base64 (std or url) encoded secret of at least 32 bytes.
func secret(k string) ([]byte, error) {
	v := str(k, "")
	if v == "" {
		return nil, fmt.Errorf("%s is required (run: go run ./cmd/app keygen)", k)
	}
	b, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		if b, err = base64.RawURLEncoding.DecodeString(v); err != nil {
			return nil, fmt.Errorf("%s must be base64", k)
		}
	}
	if len(b) < 32 {
		return nil, fmt.Errorf("%s must decode to at least 32 bytes", k)
	}
	return b, nil
}
