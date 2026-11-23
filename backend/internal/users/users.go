// Package users: profile + every data-subject right the GDPR requires
// (access/portability Art.15/20, rectification Art.16, erasure Art.17,
// consent withdrawal Art.7(3)) and session management.
package users

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"app/internal/platform/audit"
	"app/internal/platform/httpx"
	"app/internal/platform/jobs"
	"app/internal/platform/kit"
)

// Purposes a user may toggle. Add yours (e.g. "personalisation") and show them in the app's privacy screen.
var togglePurposes = map[string]bool{"marketing": true, "analytics": true, "crash_reports": true}

type Module struct{ d *kit.Deps }

func Register(r fiber.Router, d *kit.Deps) {
	m := &Module{d}
	g := r // mounted at /v1/me by server.go
	g.Get("/", m.me)
	g.Patch("/", m.update)
	g.Get("/consents", m.consents)
	g.Put("/consents", m.setConsent)
	g.Get("/export", limiter.New(limiter.Config{
		Max: 3, Expiration: time.Hour,
		KeyGenerator: func(c fiber.Ctx) string { return "export:" + httpx.UserID(c).String() },
		LimitReached: func(fiber.Ctx) error { return httpx.ErrRateLimited },
	}), m.export)
	g.Delete("/", m.requestDeletion)
	g.Get("/sessions", m.sessions)
	g.Delete("/sessions", m.logoutAll)
	g.Delete("/sessions/:id", m.revokeSession)
}

type profile struct {
	ID            uuid.UUID       `json:"id"`
	Email         *string         `json:"email"`
	EmailVerified bool            `json:"email_verified"`
	Phone         *string         `json:"phone"` // masked
	Name          string          `json:"name"`
	Locale        string          `json:"locale"`
	Role          string          `json:"role"`
	CreatedAt     time.Time       `json:"created_at"`
	Consents      map[string]bool `json:"consents"`
}

func (m *Module) me(c fiber.Ctx) error {
	p, err := m.load(c, httpx.UserID(c))
	if err != nil {
		return err
	}
	return c.JSON(p)
}

func (m *Module) load(ctx context.Context, uid uuid.UUID) (profile, error) {
	var p profile
	var verifiedAt *time.Time
	var phoneEnc []byte
	err := m.d.DB.QueryRow(ctx, `SELECT id, email, email_verified_at, phone_enc, name, locale, role, created_at
		FROM users WHERE id=$1 AND status='active'`, uid).
		Scan(&p.ID, &p.Email, &verifiedAt, &phoneEnc, &p.Name, &p.Locale, &p.Role, &p.CreatedAt)
	if err == pgx.ErrNoRows {
		return p, httpx.ErrUnauthorized
	}
	if err != nil {
		return p, err
	}
	p.EmailVerified = verifiedAt != nil
	if phoneEnc != nil {
		if pt, err := m.d.Keys.Decrypt(phoneEnc); err == nil {
			masked := kit.MaskPhone(string(pt))
			p.Phone = &masked
		}
	}
	p.Consents, err = m.currentConsents(ctx, uid)
	return p, err
}

func (m *Module) update(c fiber.Ctx) error {
	var req struct {
		Name   *string `json:"name" validate:"omitempty,max=100"`
		Locale *string `json:"locale" validate:"omitempty,min=2,max=10"`
	}
	if err := httpx.Bind(c, &req); err != nil {
		return err
	}
	uid := httpx.UserID(c)
	if _, err := m.d.DB.Exec(c, `UPDATE users SET name=coalesce($2,name), locale=coalesce($3,locale) WHERE id=$1`,
		uid, req.Name, req.Locale); err != nil {
		return err
	}
	return m.me(c)
}

// ---- consent ----------------------------------------------------------------

func (m *Module) currentConsents(ctx context.Context, uid uuid.UUID) (map[string]bool, error) {
	rows, err := m.d.DB.Query(ctx, `SELECT DISTINCT ON (purpose) purpose, granted FROM consents
		WHERE user_id=$1 ORDER BY purpose, created_at DESC`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for k := range togglePurposes {
		out[k] = false // default OFF: consent must be opt-in (GDPR Art.4(11), Planet49)
	}
	for rows.Next() {
		var p string
		var g bool
		if err := rows.Scan(&p, &g); err != nil {
			return nil, err
		}
		out[p] = g
	}
	return out, rows.Err()
}

func (m *Module) consents(c fiber.Ctx) error {
	cs, err := m.currentConsents(c, httpx.UserID(c))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"consents": cs, "policy_version": m.d.Cfg.PrivacyVersion})
}

func (m *Module) setConsent(c fiber.Ctx) error {
	var req struct {
		Purpose string `json:"purpose" validate:"required,max=50"`
		Granted bool   `json:"granted"`
	}
	if err := httpx.Bind(c, &req); err != nil {
		return err
	}
	if !togglePurposes[req.Purpose] {
		return httpx.Err(422, "unknown_purpose", "Unknown consent purpose")
	}
	uid := httpx.UserID(c)
	if _, err := m.d.DB.Exec(c, `INSERT INTO consents(user_id, purpose, granted, policy_version, source)
		VALUES ($1,$2,$3,$4,'settings')`, uid, req.Purpose, req.Granted, m.d.Cfg.PrivacyVersion); err != nil {
		return err
	}
	audit.Log(c, m.d.DB, m.d.Log, uid, audit.ConsentChanged, m.d.IPHash(c), map[string]any{"purpose": req.Purpose, "granted": req.Granted})
	return m.consents(c)
}

// ---- export (Art. 15 + 20: machine-readable copy) ----------------------------

// export builds a JSON document of everything stored about the user. When you
// add a table with user data, add it here AND in docs/compliance/DATA_CATALOG.md.
func (m *Module) export(c fiber.Ctx) error {
	uid := httpx.UserID(c)
	out := map[string]any{"generated_at": time.Now().UTC(), "format_version": 1}

	var phone *string
	var phoneEnc []byte
	user := map[string]any{}
	var email *string
	var name, locale, role string
	var created time.Time
	var verified, terms *time.Time
	var termsV *string
	if err := m.d.DB.QueryRow(c, `SELECT email, phone_enc, name, locale, role, created_at, email_verified_at, terms_version, terms_accepted_at
		FROM users WHERE id=$1`, uid).Scan(&email, &phoneEnc, &name, &locale, &role, &created, &verified, &termsV, &terms); err != nil {
		return err
	}
	if phoneEnc != nil {
		if pt, err := m.d.Keys.Decrypt(phoneEnc); err == nil {
			s := string(pt)
			phone = &s
		}
	}
	user["id"], user["email"], user["phone"], user["name"], user["locale"], user["role"] = uid, email, phone, name, locale, role
	user["created_at"], user["email_verified_at"], user["terms_version"], user["terms_accepted_at"] = created, verified, termsV, terms
	out["user"] = user

	sections := map[string]string{
		"consents":        `SELECT purpose, granted, policy_version, source, created_at FROM consents WHERE user_id=$1 ORDER BY created_at`,
		"sessions":        `SELECT created_at, last_used_at, expires_at, revoked_at, user_agent FROM sessions WHERE user_id=$1 ORDER BY created_at`,
		"security_events": `SELECT action, meta, created_at FROM audit_log WHERE user_id=$1 ORDER BY created_at`,
		"media":           `SELECT id, kind, original_name, content_type, size_bytes, created_at FROM media WHERE owner_id=$1 ORDER BY created_at`,
		"notes":           `SELECT id, title, body, tags, archived, created_at, updated_at FROM notes WHERE owner_id=$1 ORDER BY created_at`,
	}
	for key, q := range sections {
		rows, err := m.d.DB.Query(c, q, uid)
		if err != nil {
			return err
		}
		list, err := pgx.CollectRows(rows, pgx.RowToMap)
		if err != nil {
			return err
		}
		out[key] = list
	}
	audit.Log(c, m.d.DB, m.d.Log, uid, audit.DataExported, m.d.IPHash(c), nil)
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	c.Set(fiber.HeaderContentDisposition, fmt.Sprintf(`attachment; filename="my-data-%s.json"`, time.Now().Format("2006-01-02")))
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	return c.Send(b)
}

// ---- erasure (Art. 17) ------------------------------------------------------

func (m *Module) requestDeletion(c fiber.Ctx) error {
	var req struct {
		Confirm string `json:"confirm" validate:"required,eq=DELETE"`
	}
	if err := httpx.Bind(c, &req); err != nil {
		return err
	}
	uid := httpx.UserID(c)
	purgeAt := time.Now().Add(m.d.Cfg.DeletionGrace)
	tx, err := m.d.DB.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c) //nolint:errcheck
	var email *string
	if err := tx.QueryRow(c, `UPDATE users SET status='deleted', deleted_at=now() WHERE id=$1 RETURNING email`, uid).Scan(&email); err != nil {
		return err
	}
	if _, err := tx.Exec(c, `UPDATE sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, uid); err != nil {
		return err
	}
	if err := jobs.Enqueue(c, tx, kit.JobUserPurge, map[string]string{"user_id": uid.String()}, jobs.At(purgeAt)); err != nil {
		return err
	}
	if email != nil {
		if err := kit.EnqueueEmail(c, tx, *email, "account_deleted", map[string]any{"PurgeDate": purgeAt.Format("2 January 2006")}); err != nil {
			return err
		}
	}
	audit.Log(c, tx, m.d.Log, uid, audit.DeletionRequested, m.d.IPHash(c), nil)
	if err := tx.Commit(c); err != nil {
		return err
	}
	return c.Status(202).JSON(fiber.Map{"status": "scheduled", "purge_at": purgeAt})
}

// PurgeJob permanently erases a user after the grace period. Objects in
// storage are removed first; DB rows go via ON DELETE CASCADE.
func PurgeJob(d *kit.Deps) jobs.Handler {
	return func(ctx context.Context, payload json.RawMessage) error {
		var p struct {
			UserID uuid.UUID `json:"user_id"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return jobs.Permanent{Err: err}
		}
		return Purge(ctx, d, p.UserID, false)
	}
}

// Purge erases one user. force=true skips the "still pending deletion" check.
func Purge(ctx context.Context, d *kit.Deps, uid uuid.UUID, force bool) error {
	var deletedAt *time.Time
	if err := d.DB.QueryRow(ctx, `SELECT deleted_at FROM users WHERE id=$1`, uid).Scan(&deletedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil // already gone
		}
		return err
	}
	if !force && (deletedAt == nil || time.Since(*deletedAt) < d.Cfg.DeletionGrace-time.Minute) {
		return nil // user cancelled by signing in
	}
	if d.Store != nil {
		rows, err := d.DB.Query(ctx, `SELECT storage_key, variants FROM media WHERE owner_id=$1`, uid)
		if err != nil {
			return err
		}
		type v struct {
			Key string `json:"key"`
		}
		var keys []string
		for rows.Next() {
			var k string
			var variants map[string]v
			if err := rows.Scan(&k, &variants); err != nil {
				rows.Close()
				return err
			}
			keys = append(keys, k)
			for _, vv := range variants {
				keys = append(keys, vv.Key)
			}
		}
		rows.Close()
		if err := d.Store.Delete(ctx, keys...); err != nil {
			return err
		}
	}
	if _, err := d.DB.Exec(ctx, `DELETE FROM users WHERE id=$1`, uid); err != nil {
		return err
	}
	audit.Log(ctx, d.DB, d.Log, uid, audit.UserPurged, nil, nil)
	return nil
}

// Retention enforces storage limitation (GDPR Art.5(1)(e)). Runs hourly.
func Retention(d *kit.Deps) func(context.Context) error {
	return func(ctx context.Context) error {
		stmts := []string{
			`DELETE FROM otp_codes WHERE created_at < now() - interval '1 day'`,
			`DELETE FROM sessions WHERE (revoked_at IS NOT NULL OR expires_at < now()) AND last_used_at < now() - interval '30 days'`,
			`DELETE FROM idempotency_keys WHERE created_at < now() - interval '24 hours'`,
		}
		for _, s := range stmts {
			if _, err := d.DB.Exec(ctx, s); err != nil {
				return err
			}
		}
		if _, err := d.DB.Exec(ctx, `DELETE FROM audit_log WHERE created_at < now() - $1::interval`,
			fmt.Sprintf("%d seconds", int(d.Cfg.AuditRetention.Seconds()))); err != nil {
			return err
		}
		// Safety net in case a purge job was lost.
		rows, err := d.DB.Query(ctx, `SELECT id FROM users WHERE deleted_at < now() - $1::interval LIMIT 100`,
			fmt.Sprintf("%d seconds", int(d.Cfg.DeletionGrace.Seconds())))
		if err != nil {
			return err
		}
		ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := Purge(ctx, d, id, true); err != nil {
				return err
			}
		}
		return nil
	}
}

// ---- sessions ---------------------------------------------------------------

func (m *Module) sessions(c fiber.Ctx) error {
	rows, err := m.d.DB.Query(c, `SELECT id, user_agent, created_at, last_used_at FROM sessions
		WHERE user_id=$1 AND revoked_at IS NULL AND expires_at > now() ORDER BY last_used_at DESC`, httpx.UserID(c))
	if err != nil {
		return err
	}
	type s struct {
		ID        uuid.UUID `json:"id"`
		UserAgent string    `json:"user_agent"`
		CreatedAt time.Time `json:"created_at"`
		LastUsed  time.Time `json:"last_used_at"`
		Current   bool      `json:"current"`
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (s, error) {
		var x s
		err := r.Scan(&x.ID, &x.UserAgent, &x.CreatedAt, &x.LastUsed)
		return x, err
	})
	if err != nil {
		return err
	}
	// The access token carries the session it was minted with; rotations create new
	// session rows, so "current" is best-effort (most recent row).
	if len(list) > 0 {
		list[0].Current = true
	}
	return c.JSON(fiber.Map{"items": list})
}

func (m *Module) logoutAll(c fiber.Ctx) error {
	uid := httpx.UserID(c)
	if _, err := m.d.DB.Exec(c, `UPDATE sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, uid); err != nil {
		return err
	}
	audit.Log(c, m.d.DB, m.d.Log, uid, audit.LoggedOutEverywhere, m.d.IPHash(c), nil)
	return c.SendStatus(204)
}

func (m *Module) revokeSession(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return httpx.ErrNotFound
	}
	if _, err := m.d.DB.Exec(c, `UPDATE sessions SET revoked_at=now() WHERE id=$1 AND user_id=$2`, id, httpx.UserID(c)); err != nil {
		return err
	}
	return c.SendStatus(204)
}
