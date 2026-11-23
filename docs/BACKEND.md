# Backend (Go 1.26+, Fiber v3, pgx v5, PostgreSQL 18)

## Run
`task infra:up` → `task api` (migrates, serves :8080, runs jobs in-process). Emails appear at http://localhost:8025.
Config: `backend/.env` (copied from `.env.example`, which documents every variable).

## Add a module
Use the `/add-entity` skill, or by hand: copy `internal/notes`, add a migration, register in
`internal/server/server.go`. A module exposes `Register(r fiber.Router, d *kit.Deps)` and optionally job handlers.

## Patterns
- **Handler**: `var req T; httpx.Bind(c, &req)` → SQL with `$n` params → `c.JSON(...)` or `return httpx.Err(...)`.
- **Ownership**: always `WHERE owner_id = $x` with `httpx.UserID(c)`; return 404 (not 403) for others' rows.
- **Transactions**: `tx, _ := d.DB.Begin(c); defer tx.Rollback(c)`; pass `tx` to `jobs.Enqueue` and `audit.Log`
  so side effects commit atomically with the write.
- **Lists**: define a `listquery.Spec`, `listquery.Parse(c.Query, spec)`, `q.SQL(selectFrom, "owner_id = $1", uid)`,
  `listquery.MakePage`. Add an index per sort: `(owner_id, col DESC, id DESC)`.
- **Background work**: `jobs.Enqueue(ctx, q, kind, payload)`; register handler in `server.NewWorker`.
  Return `jobs.Permanent{Err}` for non-retryable failures. Periodic: `w.Every(d, name, fn)` (must be idempotent).
- **Email**: add a template to `notify.templates`, then `kit.EnqueueEmail(ctx, q, to, "template", data)`.
- **Encrypted field**: `d.Keys.Encrypt([]byte(v))` + blind index `d.Keys.Index("purpose", v)` for lookups.
- **Admin-only route**: `r.Get("/x", httpx.RequireRole("admin"), h)`.

## Notifications (frictionless onboarding)
Only fill env vars — no code changes:
| Need | Cheapest sensible choice | Env |
|---|---|---|
| Email OTP / transactional | Amazon SES (high volume), Brevo or Resend free tiers (low volume) | `EMAIL_PROVIDER=smtp`, `SMTP_*` |
| SMS OTP | Use sparingly (expensive + fraud target). Twilio, or any HTTP gateway (MSG91, Vonage, Infobip, WhatsApp BSP) | `SMS_PROVIDER=twilio\|http` |
| Push | Firebase Cloud Messaging is free — add `firebase_messaging` + a `push.send` job when needed | – |
OTP abuse protection built in: hashed codes, 10-min TTL, 5 attempts, ≤5 sends/hour/destination, 30s cooldown,
per-IP limits. For SMS also restrict destination countries in your provider console (SMS pumping fraud).

## Commands
`go run ./cmd/app secrets` (fresh keys) · `migrate` · `worker` · `purge-user <uuid>` (immediate erasure for support).

## Testing
Unit tests next to code. `internal/server/server_test.go` is an end-to-end flow against a real DB
(`task test:api` creates `app_test`). It reads OTP codes from the `jobs` table instead of an inbox.
