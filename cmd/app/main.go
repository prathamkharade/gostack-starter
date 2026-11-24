// Command app is the single binary for the whole system.
//
//	app            run web server + background worker in one process (cheapest; default)
//	app web        run only the web server (scale horizontally)
//	app worker     run only the job worker (media processing, emails…)
//	app migrate    apply database migrations and exit
//	app keygen     print fresh secrets for .env
//	app healthcheck  exit 0 if the local server answers /healthz (Docker HEALTHCHECK)
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
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
	"app/internal/web"
)

func main() {
	cmd := "all"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "keygen":
		keygen()
		return
	case "healthcheck":
		os.Exit(healthcheck())
	}
	if err := run(cmd); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(cmd string) error {
	switch cmd {
	case "all", "web", "worker", "migrate":
	default:
		return fmt.Errorf("unknown command %q (use: all | web | worker | migrate | keygen | healthcheck)", cmd)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.Open(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := database.Migrate(ctx, pool, log); err != nil {
		return err
	}
	if cmd == "migrate" {
		return nil
	}

	keys, err := security.NewKeyring(cfg.EncryptionKeys)
	if err != nil {
		return err
	}
	queue := jobs.New(pool, keys, log)

	// Notifications: providers come from config only.
	mailer := &notify.SMTPMailer{Host: cfg.SMTPHost, Port: cfg.SMTPPort, User: cfg.SMTPUser, Pass: cfg.SMTPPass, TLS: cfg.SMTPTLS, From: cfg.MailFrom}
	sms := notify.NewSMS(cfg.SMSProvider, cfg.SMSFrom, cfg.TwilioSID, cfg.TwilioToken, cfg.VonageKey, cfg.VonageSecret, log)
	queue.Register("email.send", func(ctx context.Context, p json.RawMessage) error {
		var e notify.Email
		if err := json.Unmarshal(p, &e); err != nil {
			return err
		}
		return mailer.Send(ctx, e)
	})
	queue.Register("sms.send", func(ctx context.Context, p json.RawMessage) error {
		var m notify.SMS
		if err := json.Unmarshal(p, &m); err != nil {
			return err
		}
		return sms.Send(ctx, m)
	})

	store, err := newStorage(cfg)
	if err != nil {
		return err
	}
	proc := media.NewProcessor(media.Options{
		ImageFormat: cfg.MediaImageFormat, ImageQuality: cfg.MediaImageQuality, ImageWidths: cfg.MediaImageWidths,
		VideoCodec: cfg.MediaVideoCodec, VideoMaxHeight: cfg.MediaVideoMaxHeight,
	})
	publicBase := ""
	if cfg.StorageDriver == "s3" {
		publicBase = cfg.S3PublicURL
	}
	mediaSvc := media.NewService(pool, store, proc, queue, keys, log, cfg.MediaMaxUploadMB, cfg.MediaKeepOriginal, publicBase)

	authSvc := auth.New(pool, keys, cfg, queue, log)
	privacySvc := privacy.New(pool, authSvc, queue, cfg.AppName, log)
	itemsRepo := items.NewRepo(pool)
	queue.Every("retention", time.Hour, privacySvc.Retention(cfg.RetentionAuditDays, cfg.RetentionJobsDays))

	errc := make(chan error, 2)
	if cmd == "all" || cmd == "worker" {
		log.Info("media tools", "available", proc.Tools())
		go func() { queue.Run(ctx, cfg.WorkerConcurrency); errc <- nil }()
	}
	if cmd == "all" || cmd == "web" {
		srv := web.New(web.Deps{Cfg: cfg, Log: log, Pool: pool, Auth: authSvc, Media: mediaSvc, Privacy: privacySvc, Items: itemsRepo})
		go func() {
			log.Info("listening", "addr", cfg.Addr, "env", cfg.Env)
			errc <- srv.App.Listen(cfg.Addr, fiber.ListenConfig{DisableStartupMessage: true, GracefulContext: ctx, ShutdownTimeout: 20 * time.Second})
		}()
	}
	select {
	case <-ctx.Done():
		log.Info("shutting down")
		time.Sleep(500 * time.Millisecond) // let in-flight jobs observe cancellation
		return nil
	case err := <-errc:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		<-ctx.Done()
		return nil
	}
}

func newStorage(cfg *config.Config) (storage.Storage, error) {
	if cfg.StorageDriver == "s3" {
		return &storage.S3{Endpoint: cfg.S3Endpoint, Region: cfg.S3Region, Bucket: cfg.S3Bucket, AccessKey: cfg.S3AccessKey,
			SecretKey: cfg.S3SecretKey, PublicURL: cfg.S3PublicURL, PathStyle: cfg.S3PathStyle}, nil
	}
	return storage.NewLocal(cfg.StorageLocalDir)
}

func newLogger(cfg *config.Config) *slog.Logger {
	var lvl slog.Level
	_ = lvl.UnmarshalText([]byte(strings.ToUpper(cfg.LogLevel)))
	opts := &slog.HandlerOptions{Level: lvl}
	if cfg.IsDev() {
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts)) // JSON in production: easy to ship/search
}

func keygen() {
	k := func() string {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		return base64.StdEncoding.EncodeToString(b)
	}
	fmt.Printf("JWT_SECRET=%s\nBLIND_INDEX_KEY=%s\nENCRYPTION_KEYS=v1:%s\n", k(), k(), k())
}

func healthcheck() int {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	c := http.Client{Timeout: 3 * time.Second}
	res, err := c.Get("http://" + addr + "/healthz")
	if err != nil || res.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
