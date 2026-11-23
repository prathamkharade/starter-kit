package httpx

import (
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"app/internal/platform/token"
)

type ctxKey int

const (
	keyUserID ctxKey = iota
	keyRole
	keySessionID
)

// RequestLogger logs one line per request. It never logs bodies, query
// strings or headers (they may contain personal data or secrets).
func RequestLogger(log *slog.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		status := c.Response().StatusCode()
		if err != nil {
			var ae *APIError
			if e, ok := err.(*APIError); ok {
				ae = e
				status = ae.Status
			} else if fe, ok := err.(*fiber.Error); ok {
				status = fe.Code
			} else {
				status = 500
			}
		}
		route := c.Route().Path // pattern like /v1/notes/:id, not the raw path (no ids in logs)
		lvl := slog.LevelInfo
		if status >= 500 {
			lvl = slog.LevelError
		}
		log.Log(c, lvl, "http",
			"method", c.Method(), "route", route, "status", status,
			"ms", time.Since(start).Milliseconds(), "request_id", c.RequestID())
		return err
	}
}

// Auth requires a valid Bearer access token and stores the identity in Locals.
func Auth(iss *token.Issuer) fiber.Handler {
	return func(c fiber.Ctx) error {
		h := c.Get(fiber.HeaderAuthorization)
		raw, ok := strings.CutPrefix(h, "Bearer ")
		if !ok || raw == "" {
			return ErrUnauthorized
		}
		claims, err := iss.Parse(raw)
		if err != nil {
			return ErrUnauthorized
		}
		uid, _ := uuid.Parse(claims.Subject)
		sid, _ := uuid.Parse(claims.SID)
		c.Locals(keyUserID, uid)
		c.Locals(keyRole, claims.Role)
		c.Locals(keySessionID, sid)
		return c.Next()
	}
}

// RequireRole must be chained after Auth.
func RequireRole(roles ...string) fiber.Handler {
	return func(c fiber.Ctx) error {
		r, _ := c.Locals(keyRole).(string)
		for _, want := range roles {
			if r == want {
				return c.Next()
			}
		}
		return ErrForbidden
	}
}

func UserID(c fiber.Ctx) uuid.UUID    { v, _ := c.Locals(keyUserID).(uuid.UUID); return v }
func SessionID(c fiber.Ctx) uuid.UUID { v, _ := c.Locals(keySessionID).(uuid.UUID); return v }
func Role(c fiber.Ctx) string         { v, _ := c.Locals(keyRole).(string); return v }

// Idempotency makes unsafe requests carrying an `Idempotency-Key` header safe
// to retry (the offline outbox in the Flutter app relies on this). The first
// response is stored for 24h and replayed for duplicates. Chain after Auth.
func Idempotency(pool *pgxpool.Pool) fiber.Handler {
	return func(c fiber.Ctx) error {
		key := c.Get("Idempotency-Key")
		m := c.Method()
		if key == "" || m == fiber.MethodGet || m == fiber.MethodHead || len(key) > 100 {
			return c.Next()
		}
		uid := UserID(c)
		if uid == uuid.Nil {
			return c.Next()
		}
		tag, err := pool.Exec(c, `INSERT INTO idempotency_keys(user_id, key, method, path)
			VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, uid, key, m, c.Path())
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 { // duplicate
			var status *int
			var body []byte
			err := pool.QueryRow(c, `SELECT status_code, response FROM idempotency_keys WHERE user_id=$1 AND key=$2`,
				uid, key).Scan(&status, &body)
			if err != nil && err != pgx.ErrNoRows {
				return err
			}
			if status == nil {
				return Err(409, "request_in_progress", "A request with this Idempotency-Key is still processing")
			}
			c.Set("Idempotent-Replay", "true")
			c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
			return c.Status(*status).Send(body)
		}
		if err := c.Next(); err != nil {
			// let the client retry with the same key after a failure
			_, _ = pool.Exec(c, `DELETE FROM idempotency_keys WHERE user_id=$1 AND key=$2`, uid, key)
			return err
		}
		resp := c.Response()
		_, err = pool.Exec(c, `UPDATE idempotency_keys SET status_code=$3, response=$4 WHERE user_id=$1 AND key=$2`,
			uid, key, resp.StatusCode(), append([]byte(nil), resp.Body()...))
		return err
	}
}
