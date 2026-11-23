// Package server wires middleware, routes and background jobs. This is the
// one file to read to see every endpoint and job the backend exposes.
package server

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	recoverer "github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"

	"app/internal/auth"
	"app/internal/media"
	"app/internal/notes"
	"app/internal/platform/httpx"
	"app/internal/platform/jobs"
	"app/internal/platform/kit"
	"app/internal/platform/notify"
	"app/internal/users"
)

func New(d *kit.Deps) *fiber.App {
	cfg := d.Cfg
	fc := fiber.Config{
		AppName:      cfg.AppName,
		BodyLimit:    1 << 20, // 1 MiB: JSON only — files go straight to object storage
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
		ErrorHandler: httpx.ErrorHandler(d.Log),
	}
	if cfg.TrustProxy { // behind Caddy / Cloudflare: read the real client IP
		fc.TrustProxy = true
		fc.ProxyHeader = fiber.HeaderXForwardedFor
		fc.EnableIPValidation = true
		fc.TrustProxyConfig = fiber.TrustProxyConfig{Proxies: cfg.Proxies, Loopback: true, Private: true}
	}
	app := fiber.New(fc)

	app.Use(
		recoverer.New(),
		requestid.New(),
		httpx.RequestLogger(d.Log),
		helmet.New(),
		cors.New(cors.Config{
			AllowOrigins:     cfg.CORSOrigins,
			AllowCredentials: true,
			AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowHeaders:     []string{"Authorization", "Content-Type", "Idempotency-Key", "X-Client", "X-Requested-With"},
			MaxAge:           600,
		}),
		limiter.New(limiter.Config{ // global per-IP ceiling; per-route limits are stricter
			Max: 300, Expiration: time.Minute,
			LimitReached: func(fiber.Ctx) error { return httpx.ErrRateLimited },
		}),
	)

	app.Get("/healthz", func(c fiber.Ctx) error { return c.SendString("ok") })
	app.Get("/readyz", func(c fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c, 2*time.Second)
		defer cancel()
		if err := d.DB.Ping(ctx); err != nil {
			return httpx.Err(503, "db_unavailable", "Database unavailable")
		}
		return c.SendString("ready")
	})

	if cfg.Maintenance {
		app.Use(func(c fiber.Ctx) error {
			if strings.HasPrefix(c.Path(), "/v1/app/config") {
				return c.Next()
			}
			return httpx.Err(503, "maintenance", "We are doing maintenance. Back soon.")
		})
	}

	v1 := app.Group("/v1")
	// Remote config: force-update, maintenance banner, feature flags. Cached by clients.
	v1.Get("/app/config", func(c fiber.Ctx) error {
		c.Set(fiber.HeaderCacheControl, "public, max-age=300")
		return c.JSON(fiber.Map{
			"min_version": cfg.AppMinVersion, "latest_version": cfg.AppLatestVersion,
			"maintenance": cfg.Maintenance, "sms_login": cfg.SMSProvider != "none",
			"terms_version": cfg.TermsVersion, "privacy_version": cfg.PrivacyVersion,
			"features": fiber.Map{}, // add flags here
		})
	})

	requireAuth := httpx.Auth(d.Tokens)
	idem := httpx.Idempotency(d.DB)

	auth.Register(v1, d, requireAuth)
	users.Register(v1.Group("/me", requireAuth, idem), d)
	notes.Register(v1.Group("/notes", requireAuth, idem), d)
	media.Register(v1.Group("/media", requireAuth, idem), d)
	// ↑ register new modules here: x.Register(v1.Group("/x", requireAuth, idem), d)

	app.Use(func(fiber.Ctx) error { return httpx.ErrNotFound })
	return app
}

// NewWorker registers every background job and periodic task.
func NewWorker(d *kit.Deps, email notify.EmailSender, sms notify.SMSSender) *jobs.Worker {
	w := jobs.NewWorker(d.DB, d.Log, d.Cfg.WorkerConcurrency)

	w.Handle(kit.JobEmail, func(ctx context.Context, p json.RawMessage) error {
		var j kit.EmailJob
		if err := json.Unmarshal(p, &j); err != nil {
			return jobs.Permanent{Err: err}
		}
		e, err := notify.Render(d.Cfg.AppName, j.Template, j.Data)
		if err != nil {
			return jobs.Permanent{Err: err}
		}
		e.To = j.To
		return email.Send(ctx, e)
	})
	w.Handle(kit.JobSMS, func(ctx context.Context, p json.RawMessage) error {
		var j kit.SMSJob
		if err := json.Unmarshal(p, &j); err != nil {
			return jobs.Permanent{Err: err}
		}
		return sms.Send(ctx, j.To, j.Body)
	})
	if d.Store != nil {
		w.Handle(kit.JobMediaProcess, media.ProcessJob(d))
	}
	w.Handle(kit.JobUserPurge, users.PurgeJob(d))

	w.Every(time.Hour, "privacy.retention", users.Retention(d))
	w.Every(10*time.Minute, "jobs.maintenance", jobs.Maintenance(d.DB))
	return w
}
