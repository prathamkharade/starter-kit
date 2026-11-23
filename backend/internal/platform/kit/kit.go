// Package kit bundles the shared dependencies every module receives, plus
// small helpers used across modules. Modules depend on kit, never on each other.
package kit

import (
	"context"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgxpool"

	"app/internal/config"
	"app/internal/platform/db"
	"app/internal/platform/jobs"
	"app/internal/platform/secure"
	"app/internal/platform/storage"
	"app/internal/platform/token"
)

type Deps struct {
	Cfg    *config.Config
	DB     *pgxpool.Pool
	Log    *slog.Logger
	Tokens *token.Issuer
	Keys   *secure.Keyring
	Store  *storage.Store // nil when object storage is not configured
}

// IPHash pseudonymises the client IP (GDPR Recital 26/28) for security logs.
func (d *Deps) IPHash(c fiber.Ctx) []byte { return d.Keys.Index("ip", c.IP())[:16] }

// Job kinds handled by the worker (registered in internal/server/server.go).
const (
	JobEmail        = "email.send"
	JobSMS          = "sms.send"
	JobMediaProcess = "media.process"
	JobUserPurge    = "user.purge"
)

type EmailJob struct {
	To       string         `json:"to"`
	Template string         `json:"template"`
	Data     map[string]any `json:"data"`
}

type SMSJob struct {
	To   string `json:"to"`
	Body string `json:"body"`
}

func EnqueueEmail(ctx context.Context, q db.Querier, to, template string, data map[string]any) error {
	return jobs.Enqueue(ctx, q, JobEmail, EmailJob{To: to, Template: template, Data: data})
}

func EnqueueSMS(ctx context.Context, q db.Querier, to, body string) error {
	return jobs.Enqueue(ctx, q, JobSMS, SMSJob{To: to, Body: body}, jobs.MaxAttempts(3))
}

func NormalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// NormalizePhone returns E.164 (+491701234567) or "" if invalid.
func NormalizePhone(s string) string {
	var b strings.Builder
	for i, r := range strings.TrimSpace(s) {
		switch {
		case r == '+' && i == 0:
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '(' || r == ')' || r == '.':
		default:
			return ""
		}
	}
	p := b.String()
	if len(p) < 8 || len(p) > 16 || p[0] != '+' || p[1] == '0' {
		return ""
	}
	return p
}

// MaskPhone shows only the last 3 digits (+49•••••567).
func MaskPhone(p string) string {
	if len(p) < 6 {
		return "•••"
	}
	return p[:3] + strings.Repeat("•", len(p)-6) + p[len(p)-3:]
}
