// Package auth: registration, email/phone OTP (passwordless), password login,
// refresh-token rotation with reuse detection, password reset, logout.
// See docs/SECURITY.md#authentication for the threat model.
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"app/internal/platform/audit"
	"app/internal/platform/db"
	"app/internal/platform/httpx"
	"app/internal/platform/kit"
	"app/internal/platform/secure"
)

const (
	otpTTL         = 10 * time.Minute
	otpMaxAttempts = 5
	maxFailedLogin = 5
	lockDuration   = 15 * time.Minute
	refreshCookie  = "rt"
)

var (
	errInvalidCode = httpx.Err(400, "invalid_code", "The code is invalid or has expired")
	errInvalidCred = httpx.Err(401, "invalid_credentials", "Email or password is incorrect")
	errSession     = httpx.Err(401, "session_expired", "Please sign in again")
)

type Module struct{ d *kit.Deps }

func Register(r fiber.Router, d *kit.Deps, requireAuth fiber.Handler) {
	m := &Module{d}
	strict := limiter.New(limiter.Config{
		Max: 10, Expiration: time.Minute,
		LimitReached: func(fiber.Ctx) error { return httpx.ErrRateLimited },
	})
	g := r.Group("/auth")
	g.Post("/register", strict, m.register)
	g.Post("/verify-email", strict, m.verifyEmail)
	g.Post("/login", strict, m.login)
	g.Post("/otp/request", strict, m.otpRequest)
	g.Post("/otp/verify", strict, m.otpVerify)
	g.Post("/password/forgot", strict, m.forgot)
	g.Post("/password/reset", strict, m.reset)
	g.Post("/refresh", m.refresh)
	g.Post("/logout", requireAuth, m.logout)
}

// ---- DTOs -------------------------------------------------------------------

type userBrief struct {
	ID    uuid.UUID `json:"id"`
	Email *string   `json:"email,omitempty"`
	Name  string    `json:"name"`
	Role  string    `json:"role"`
}

type tokenResponse struct {
	AccessToken  string    `json:"access_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	RefreshToken string    `json:"refresh_token,omitempty"` // only for X-Client: mobile
	User         userBrief `json:"user"`
}

// ---- register / verify -------------------------------------------------------

func (m *Module) register(c fiber.Ctx) error {
	var req struct {
		Email            string `json:"email" validate:"required,email,max=254"`
		Password         string `json:"password" validate:"required,min=10,max=128"`
		Name             string `json:"name" validate:"max=100"`
		Locale           string `json:"locale" validate:"omitempty,max=10"`
		AcceptTerms      bool   `json:"accept_terms"`
		MarketingConsent bool   `json:"marketing_consent"`
	}
	if err := httpx.Bind(c, &req); err != nil {
		return err
	}
	if !req.AcceptTerms {
		return &httpx.APIError{Status: 422, Code: "terms_required", Message: "Please accept the terms to continue",
			Fields: map[string]string{"accept_terms": "required"}}
	}
	if req.Locale == "" {
		req.Locale = "en"
	}
	email := kit.NormalizeEmail(req.Email)
	hash, err := secure.HashPassword(req.Password)
	if err != nil {
		return err
	}
	tx, err := m.d.DB.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c) //nolint:errcheck

	var id uuid.UUID
	err = tx.QueryRow(c, `INSERT INTO users(email, password_hash, name, locale, terms_version, terms_accepted_at)
		VALUES ($1,$2,$3,$4,$5,now()) ON CONFLICT (email) DO NOTHING RETURNING id`,
		email, hash, req.Name, req.Locale, m.d.Cfg.TermsVersion).Scan(&id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Account exists. Respond identically (no account enumeration); resend code if still unverified.
		var verified *time.Time
		if tx.QueryRow(c, `SELECT email_verified_at FROM users WHERE email=$1`, email).Scan(&verified) == nil && verified == nil {
			if err := m.sendOTP(c, tx, "verify_email", "email", email); err != nil {
				return err
			}
		}
	case err != nil:
		return err
	default:
		if err := m.recordSignupConsents(c, tx, id, req.MarketingConsent, "signup"); err != nil {
			return err
		}
		audit.Log(c, tx, m.d.Log, id, audit.UserRegistered, m.d.IPHash(c), map[string]any{"method": "password"})
		if err := m.sendOTP(c, tx, "verify_email", "email", email); err != nil {
			return err
		}
	}
	if err := tx.Commit(c); err != nil {
		return err
	}
	return c.Status(202).JSON(fiber.Map{"status": "verification_sent"})
}

func (m *Module) verifyEmail(c fiber.Ctx) error {
	var req struct {
		Email string `json:"email" validate:"required,email"`
		Code  string `json:"code" validate:"required,len=6,numeric"`
	}
	if err := httpx.Bind(c, &req); err != nil {
		return err
	}
	email := kit.NormalizeEmail(req.Email)
	if err := m.checkOTP(c, "verify_email", "email", email, req.Code); err != nil {
		return err
	}
	var u userBrief
	err := m.d.DB.QueryRow(c, `UPDATE users SET email_verified_at=coalesce(email_verified_at, now())
		WHERE email=$1 AND status <> 'suspended' RETURNING id, email, name, role`, email).Scan(&u.ID, &u.Email, &u.Name, &u.Role)
	if err != nil {
		return errInvalidCode
	}
	return m.startSession(c, u, uuid.New())
}

// ---- password login ---------------------------------------------------------

func (m *Module) login(c fiber.Ctx) error {
	var req struct {
		Email    string `json:"email" validate:"required,email"`
		Password string `json:"password" validate:"required,max=128"`
	}
	if err := httpx.Bind(c, &req); err != nil {
		return err
	}
	email := kit.NormalizeEmail(req.Email)
	var (
		u        userBrief
		hash     *string
		verified *time.Time
		locked   *time.Time
		status   string
	)
	err := m.d.DB.QueryRow(c, `SELECT id, email, name, role, password_hash, email_verified_at, locked_until, status
		FROM users WHERE email=$1`, email).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &hash, &verified, &locked, &status)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && hash == nil) {
		secure.DummyVerify(req.Password) // equalise timing
		return errInvalidCred
	}
	if err != nil {
		return err
	}
	if locked != nil && locked.After(time.Now()) {
		return httpx.Err(429, "account_locked", "Too many failed attempts. Try again in a few minutes or reset your password.")
	}
	ok, err := secure.VerifyPassword(req.Password, *hash)
	if err != nil {
		return err
	}
	if !ok {
		var n int
		_ = m.d.DB.QueryRow(c, `UPDATE users SET failed_logins = failed_logins + 1,
			locked_until = CASE WHEN failed_logins + 1 >= $2 THEN now() + $3::interval ELSE locked_until END
			WHERE id=$1 RETURNING failed_logins`, u.ID, maxFailedLogin, fmt.Sprintf("%d seconds", int(lockDuration.Seconds()))).Scan(&n)
		audit.Log(c, m.d.DB, m.d.Log, u.ID, audit.LoginFailed, m.d.IPHash(c), nil)
		if n >= maxFailedLogin {
			audit.Log(c, m.d.DB, m.d.Log, u.ID, audit.AccountLocked, m.d.IPHash(c), nil)
		}
		return errInvalidCred
	}
	if status == "suspended" {
		return httpx.Err(403, "account_suspended", "This account has been suspended")
	}
	if verified == nil {
		_ = m.sendOTP(c, m.d.DB, "verify_email", "email", email)
		return httpx.Err(403, "email_not_verified", "Please verify your email. We sent you a new code.")
	}
	if _, err := m.d.DB.Exec(c, `UPDATE users SET failed_logins=0, locked_until=NULL WHERE id=$1`, u.ID); err != nil {
		return err
	}
	if err := m.reactivateIfPendingDeletion(c, u.ID, status); err != nil {
		return err
	}
	audit.Log(c, m.d.DB, m.d.Log, u.ID, audit.LoginSucceeded, m.d.IPHash(c), map[string]any{"method": "password"})
	return m.startSession(c, u, uuid.New())
}

// ---- passwordless OTP (email or SMS) — "frictionless onboarding" -----------

func (m *Module) otpRequest(c fiber.Ctx) error {
	var req struct {
		Channel     string `json:"channel" validate:"required,oneof=email sms"`
		Destination string `json:"destination" validate:"required,max=254"`
	}
	if err := httpx.Bind(c, &req); err != nil {
		return err
	}
	dest, err := normalizeDest(req.Channel, req.Destination)
	if err != nil {
		return err
	}
	if req.Channel == "sms" && m.d.Cfg.SMSProvider == "none" {
		return httpx.Err(422, "sms_disabled", "SMS sign-in is not available")
	}
	if !m.d.Cfg.SignupViaOTP {
		if _, err := m.findByDest(c, req.Channel, dest); err != nil {
			return c.Status(202).JSON(fiber.Map{"status": "code_sent"}) // same response: no enumeration
		}
	}
	if err := m.sendOTP(c, m.d.DB, "login", req.Channel, dest); err != nil {
		return err
	}
	return c.Status(202).JSON(fiber.Map{"status": "code_sent"})
}

func (m *Module) otpVerify(c fiber.Ctx) error {
	var req struct {
		Channel     string `json:"channel" validate:"required,oneof=email sms"`
		Destination string `json:"destination" validate:"required,max=254"`
		Code        string `json:"code" validate:"required,len=6,numeric"`
	}
	if err := httpx.Bind(c, &req); err != nil {
		return err
	}
	dest, err := normalizeDest(req.Channel, req.Destination)
	if err != nil {
		return err
	}
	if err := m.checkOTP(c, "login", req.Channel, dest, req.Code); err != nil {
		return err
	}
	u, err := m.findByDest(c, req.Channel, dest)
	if errors.Is(err, pgx.ErrNoRows) {
		if !m.d.Cfg.SignupViaOTP {
			return errInvalidCode
		}
		u, err = m.createPasswordless(c, req.Channel, dest)
	}
	if err != nil {
		return err
	}
	var status string
	col := "email_verified_at"
	if req.Channel == "sms" {
		col = "phone_verified_at"
	}
	if err := m.d.DB.QueryRow(c, `UPDATE users SET `+col+`=coalesce(`+col+`, now()) WHERE id=$1 RETURNING status`, u.ID).Scan(&status); err != nil {
		return err
	}
	if status == "suspended" {
		return httpx.Err(403, "account_suspended", "This account has been suspended")
	}
	if err := m.reactivateIfPendingDeletion(c, u.ID, status); err != nil {
		return err
	}
	audit.Log(c, m.d.DB, m.d.Log, u.ID, audit.LoginSucceeded, m.d.IPHash(c), map[string]any{"method": "otp_" + req.Channel})
	return m.startSession(c, u, uuid.New())
}

func (m *Module) findByDest(ctx context.Context, channel, dest string) (userBrief, error) {
	var u userBrief
	q := `SELECT id, email, name, role FROM users WHERE email=$1`
	var arg any = dest
	if channel == "sms" {
		q = `SELECT id, email, name, role FROM users WHERE phone_hash=$1`
		arg = m.d.Keys.Index("phone", dest)
	}
	err := m.d.DB.QueryRow(ctx, q, arg).Scan(&u.ID, &u.Email, &u.Name, &u.Role)
	return u, err
}

func (m *Module) createPasswordless(ctx context.Context, channel, dest string) (userBrief, error) {
	tx, err := m.d.DB.Begin(ctx)
	if err != nil {
		return userBrief{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var u userBrief
	if channel == "email" {
		err = tx.QueryRow(ctx, `INSERT INTO users(email, terms_version, terms_accepted_at) VALUES ($1,$2,now())
			ON CONFLICT DO NOTHING RETURNING id, email, name, role`, dest, m.d.Cfg.TermsVersion).Scan(&u.ID, &u.Email, &u.Name, &u.Role)
	} else {
		enc, encErr := m.d.Keys.Encrypt([]byte(dest))
		if encErr != nil {
			return u, encErr
		}
		err = tx.QueryRow(ctx, `INSERT INTO users(phone_enc, phone_hash, terms_version, terms_accepted_at) VALUES ($1,$2,$3,now())
			ON CONFLICT DO NOTHING RETURNING id, email, name, role`, enc, m.d.Keys.Index("phone", dest), m.d.Cfg.TermsVersion).Scan(&u.ID, &u.Email, &u.Name, &u.Role)
	}
	if errors.Is(err, pgx.ErrNoRows) { // concurrent signup won the race
		return m.findByDest(ctx, channel, dest)
	}
	if err != nil {
		return u, err
	}
	if err := m.recordSignupConsents(ctx, tx, u.ID, false, "otp_signup"); err != nil {
		return u, err
	}
	audit.Log(ctx, tx, m.d.Log, u.ID, audit.UserRegistered, nil, map[string]any{"method": "otp_" + channel})
	return u, tx.Commit(ctx)
}

// ---- password reset ---------------------------------------------------------

func (m *Module) forgot(c fiber.Ctx) error {
	var req struct {
		Email string `json:"email" validate:"required,email"`
	}
	if err := httpx.Bind(c, &req); err != nil {
		return err
	}
	email := kit.NormalizeEmail(req.Email)
	if _, err := m.findByDest(c, "email", email); err == nil {
		if err := m.sendOTP(c, m.d.DB, "reset_password", "email", email); err != nil {
			return err
		}
	}
	return c.Status(202).JSON(fiber.Map{"status": "code_sent"})
}

func (m *Module) reset(c fiber.Ctx) error {
	var req struct {
		Email       string `json:"email" validate:"required,email"`
		Code        string `json:"code" validate:"required,len=6,numeric"`
		NewPassword string `json:"new_password" validate:"required,min=10,max=128"`
	}
	if err := httpx.Bind(c, &req); err != nil {
		return err
	}
	email := kit.NormalizeEmail(req.Email)
	if err := m.checkOTP(c, "reset_password", "email", email, req.Code); err != nil {
		return err
	}
	hash, err := secure.HashPassword(req.NewPassword)
	if err != nil {
		return err
	}
	var id uuid.UUID
	if err := m.d.DB.QueryRow(c, `UPDATE users SET password_hash=$2, failed_logins=0, locked_until=NULL,
		email_verified_at=coalesce(email_verified_at, now()) WHERE email=$1 RETURNING id`, email, hash).Scan(&id); err != nil {
		return errInvalidCode
	}
	if _, err := m.d.DB.Exec(c, `UPDATE sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, id); err != nil {
		return err
	}
	audit.Log(c, m.d.DB, m.d.Log, id, audit.PasswordReset, m.d.IPHash(c), nil)
	return c.SendStatus(204)
}

// ---- sessions ---------------------------------------------------------------

func (m *Module) refresh(c fiber.Ctx) error {
	raw := c.Cookies(refreshCookie)
	if raw == "" {
		var body struct {
			RefreshToken string `json:"refresh_token"`
		}
		_ = c.Bind().Body(&body)
		raw = body.RefreshToken
	}
	if raw == "" || len(raw) > 200 {
		return errSession
	}
	tx, err := m.d.DB.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c) //nolint:errcheck

	var (
		sid, uid, family uuid.UUID
		expires          time.Time
		revoked          *time.Time
	)
	err = tx.QueryRow(c, `SELECT id, user_id, family_id, expires_at, revoked_at FROM sessions
		WHERE token_hash=$1 FOR UPDATE`, secure.SHA256([]byte(raw))).Scan(&sid, &uid, &family, &expires, &revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return errSession
	}
	if err != nil {
		return err
	}
	if revoked != nil {
		if time.Since(*revoked) < 20*time.Second {
			// Two tabs refreshed at once; the other one won. Client retries once with the new cookie.
			return httpx.Err(409, "refresh_race", "Refresh already in progress, retry")
		}
		// A rotated token was replayed: likely theft. Kill the whole chain.
		_, _ = tx.Exec(c, `UPDATE sessions SET revoked_at=now() WHERE family_id=$1 AND revoked_at IS NULL`, family)
		audit.Log(c, tx, m.d.Log, uid, audit.SessionReuse, m.d.IPHash(c), nil)
		_ = tx.Commit(c)
		return errSession
	}
	if time.Now().After(expires) {
		return errSession
	}
	var u userBrief
	var status string
	if err := tx.QueryRow(c, `SELECT id, email, name, role, status FROM users WHERE id=$1`, uid).
		Scan(&u.ID, &u.Email, &u.Name, &u.Role, &status); err != nil || status != "active" {
		return errSession
	}
	if _, err := tx.Exec(c, `UPDATE sessions SET revoked_at=now(), last_used_at=now() WHERE id=$1`, sid); err != nil {
		return err
	}
	resp, rt, exp, err := m.newSession(c, tx, u, family)
	if err != nil {
		return err
	}
	if err := tx.Commit(c); err != nil {
		return err
	}
	return m.respond(c, resp, rt, exp)
}

func (m *Module) logout(c fiber.Ctx) error {
	if _, err := m.d.DB.Exec(c, `UPDATE sessions SET revoked_at=now() WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`,
		httpx.SessionID(c), httpx.UserID(c)); err != nil {
		return err
	}
	m.setCookie(c, "", time.Unix(0, 0))
	return c.SendStatus(204)
}

func (m *Module) startSession(c fiber.Ctx, u userBrief, family uuid.UUID) error {
	resp, rt, exp, err := m.newSession(c, m.d.DB, u, family)
	if err != nil {
		return err
	}
	return m.respond(c, resp, rt, exp)
}

func (m *Module) newSession(c fiber.Ctx, q db.Querier, u userBrief, family uuid.UUID) (tokenResponse, string, time.Time, error) {
	rt := secure.RandomToken(32)
	exp := time.Now().Add(m.d.Cfg.RefreshTTL)
	ua := c.Get(fiber.HeaderUserAgent)
	if len(ua) > 200 {
		ua = ua[:200]
	}
	var sid uuid.UUID
	if err := q.QueryRow(c, `INSERT INTO sessions(user_id, family_id, token_hash, user_agent, ip_hash, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, u.ID, family, secure.SHA256([]byte(rt)), ua, m.d.IPHash(c), exp).Scan(&sid); err != nil {
		return tokenResponse{}, "", exp, err
	}
	access, accessExp, err := m.d.Tokens.Issue(u.ID, sid, u.Role)
	return tokenResponse{AccessToken: access, ExpiresAt: accessExp, User: u}, rt, exp, err
}

// respond: web gets the refresh token only as an HttpOnly cookie (unreadable by
// JS → XSS-safe); mobile (X-Client: mobile) gets it in the body for secure storage.
func (m *Module) respond(c fiber.Ctx, resp tokenResponse, rt string, exp time.Time) error {
	m.setCookie(c, rt, exp)
	if c.Get("X-Client") == "mobile" {
		resp.RefreshToken = rt
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(resp)
}

func (m *Module) setCookie(c fiber.Ctx, v string, exp time.Time) {
	c.Cookie(&fiber.Cookie{
		Name: refreshCookie, Value: v, Path: "/v1/auth", Expires: exp,
		HTTPOnly: true, Secure: m.d.Cfg.IsProd(), SameSite: fiber.CookieSameSiteStrictMode,
	})
}

// ---- OTP primitives ---------------------------------------------------------

// sendOTP stores a hashed code and enqueues delivery. Silently throttles
// (max 5/hour, 1 per 30s per destination) to stop SMS-pumping fraud and spam.
func (m *Module) sendOTP(ctx context.Context, q db.Querier, purpose, channel, dest string) error {
	destHash := m.d.Keys.Index("dest", channel, dest)
	var n int
	var last *time.Time
	if err := q.QueryRow(ctx, `SELECT count(*), max(created_at) FROM otp_codes
		WHERE destination_hash=$1 AND created_at > now() - interval '1 hour'`, destHash).Scan(&n, &last); err != nil {
		return err
	}
	if n >= 5 || (last != nil && time.Since(*last) < 30*time.Second) {
		m.d.Log.Warn("otp throttled", "purpose", purpose, "channel", channel)
		return nil
	}
	code := secure.RandomDigits(6)
	if _, err := q.Exec(ctx, `INSERT INTO otp_codes(purpose, destination_hash, code_hash, expires_at)
		VALUES ($1,$2,$3,$4)`, purpose, destHash, m.d.Keys.Index("otp", purpose, dest, code), time.Now().Add(otpTTL)); err != nil {
		return err
	}
	if channel == "sms" {
		return kit.EnqueueSMS(ctx, q, dest, fmt.Sprintf("%s is your %s code. It expires in 10 minutes.", code, m.d.Cfg.AppName))
	}
	return kit.EnqueueEmail(ctx, q, dest, "otp", map[string]any{"Code": code, "Minutes": int(otpTTL.Minutes())})
}

func (m *Module) checkOTP(ctx context.Context, purpose, channel, dest, code string) error {
	var (
		id       uuid.UUID
		hash     []byte
		attempts int
		expires  time.Time
	)
	err := m.d.DB.QueryRow(ctx, `SELECT id, code_hash, attempts, expires_at FROM otp_codes
		WHERE purpose=$1 AND destination_hash=$2 AND consumed_at IS NULL
		ORDER BY created_at DESC LIMIT 1`, purpose, m.d.Keys.Index("dest", channel, dest)).Scan(&id, &hash, &attempts, &expires)
	if err != nil || time.Now().After(expires) || attempts >= otpMaxAttempts {
		return errInvalidCode
	}
	if !secure.Equal(hash, m.d.Keys.Index("otp", purpose, dest, code)) {
		_, _ = m.d.DB.Exec(ctx, `UPDATE otp_codes SET attempts=attempts+1 WHERE id=$1`, id)
		return errInvalidCode
	}
	tag, err := m.d.DB.Exec(ctx, `UPDATE otp_codes SET consumed_at=now() WHERE id=$1 AND consumed_at IS NULL`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 { // used concurrently
		return errInvalidCode
	}
	return nil
}

func (m *Module) recordSignupConsents(ctx context.Context, q db.Querier, uid uuid.UUID, marketing bool, source string) error {
	_, err := q.Exec(ctx, `INSERT INTO consents(user_id, purpose, granted, policy_version, source) VALUES
		($1,'terms',true,$2,$5), ($1,'privacy_notice',true,$3,$5), ($1,'marketing',$4,$3,$5)`,
		uid, m.d.Cfg.TermsVersion, m.d.Cfg.PrivacyVersion, marketing, source)
	return err
}

// Signing in during the deletion grace period cancels the deletion (users are told this by email).
func (m *Module) reactivateIfPendingDeletion(ctx context.Context, uid uuid.UUID, status string) error {
	if status != "deleted" {
		return nil
	}
	if _, err := m.d.DB.Exec(ctx, `UPDATE users SET status='active', deleted_at=NULL WHERE id=$1`, uid); err != nil {
		return err
	}
	audit.Log(ctx, m.d.DB, m.d.Log, uid, audit.DeletionCancelled, nil, nil)
	return nil
}

func normalizeDest(channel, raw string) (string, error) {
	if channel == "sms" {
		if p := kit.NormalizePhone(raw); p != "" {
			return p, nil
		}
		return "", &httpx.APIError{Status: 422, Code: "validation_failed", Message: "Enter the phone number with country code, e.g. +49 170 1234567",
			Fields: map[string]string{"destination": "e164"}}
	}
	e := kit.NormalizeEmail(raw)
	if err := httpx.Validate(struct {
		E string `validate:"email"`
	}{e}); err != nil {
		return "", &httpx.APIError{Status: 422, Code: "validation_failed", Message: "Enter a valid email address",
			Fields: map[string]string{"destination": "email"}}
	}
	return e, nil
}
