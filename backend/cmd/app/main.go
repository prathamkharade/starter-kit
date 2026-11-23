// Command app is the single backend binary.
//
//	app serve        HTTP API (+ embedded worker when WORKER_EMBEDDED=true)
//	app worker       background jobs only (media processing, email, SMS, retention)
//	app migrate      apply database migrations and exit
//	app secrets      print fresh secrets for .env
//	app purge-user <uuid>   immediately erase a user (support / DSAR tooling)
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"app/internal/config"
	"app/internal/platform/db"
	"app/internal/platform/kit"
	"app/internal/platform/notify"
	"app/internal/platform/secure"
	"app/internal/platform/storage"
	"app/internal/platform/token"
	"app/internal/server"
	"app/internal/users"
)

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	if cmd == "secrets" {
		printSecrets()
		return
	}
	if err := run(cmd); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run(cmd string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		return err
	}
	defer pool.Close()

	if cmd == "migrate" || cmd == "serve" {
		if err := db.Migrate(ctx, pool, log); err != nil {
			return err
		}
		if cmd == "migrate" {
			return nil
		}
	}

	keys, err := secure.NewKeyring(cfg.DataEncKey, cfg.DataIndexKey)
	if err != nil {
		return err
	}
	d := &kit.Deps{Cfg: cfg, DB: pool, Log: log, Keys: keys,
		Tokens: token.NewIssuer(cfg.JWTSecret, cfg.JWTIssuer, cfg.AccessTTL)}
	if cfg.S3AccessKey != "" {
		if d.Store, err = storage.New(cfg); err != nil {
			return err
		}
		if !cfg.IsProd() {
			if err := d.Store.EnsureBucket(ctx); err != nil {
				log.Warn("object storage unreachable; media endpoints will fail", "err", err)
			}
		}
	} else {
		log.Warn("S3_ACCESS_KEY not set: media uploads disabled")
	}

	switch cmd {
	case "purge-user":
		if len(os.Args) < 3 {
			return fmt.Errorf("usage: app purge-user <uuid>")
		}
		id, err := uuid.Parse(os.Args[2])
		if err != nil {
			return err
		}
		return users.Purge(ctx, d, id, true)
	case "worker":
		w, err := buildWorker(d)
		if err != nil {
			return err
		}
		w.Run(ctx)
		return nil
	case "serve":
		if cfg.WorkerEmbedded {
			w, err := buildWorker(d)
			if err != nil {
				return err
			}
			go w.Run(ctx)
		}
		app := server.New(d)
		errc := make(chan error, 1)
		go func() {
			log.Info("listening", "addr", cfg.HTTPAddr, "env", cfg.Env)
			errc <- app.Listen(cfg.HTTPAddr, fiber.ListenConfig{DisableStartupMessage: true})
		}()
		select {
		case err := <-errc:
			return err
		case <-ctx.Done():
		}
		log.Info("shutting down")
		sctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return app.ShutdownWithContext(sctx)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func buildWorker(d *kit.Deps) (interface{ Run(context.Context) }, error) {
	email, err := notify.NewEmailSender(d.Cfg, d.Log)
	if err != nil {
		return nil, err
	}
	sms, err := notify.NewSMSSender(d.Cfg, d.Log)
	if err != nil {
		return nil, err
	}
	return server.NewWorker(d, email, sms), nil
}

func newLogger(cfg *config.Config) *slog.Logger {
	lvl := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		lvl = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: lvl}
	if cfg.IsProd() {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

func printSecrets() {
	b := func(n int) string {
		k := make([]byte, n)
		if _, err := rand.Read(k); err != nil {
			panic(err)
		}
		return base64.StdEncoding.EncodeToString(k)
	}
	fmt.Printf("JWT_SECRET=%s\nDATA_ENC_KEY=%s\nDATA_INDEX_KEY=%s\n", b(48), b(32), b(32))
}
