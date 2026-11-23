# CLAUDE.md — read first, then load ONLY the doc you need

Monorepo: Go API (`backend/`), Flutter web+mobile (`frontend/`), Docker/Caddy infra (`infra/`).
App-specific facts live in `docs/APP.md` (created by `/new-app`). Keep this file short.

## Where things are (open only what the task needs)
| Task | Read |
|---|---|
| Add an entity/feature end-to-end | `/add-entity` skill → copies `backend/internal/notes` + `frontend/lib/features/notes` |
| Endpoints, error envelope, list query contract | `docs/API.md` |
| Backend structure, jobs, config | `docs/BACKEND.md` |
| Flutter structure, theme, offline, web perf | `docs/FRONTEND.md` |
| Personal data, consent, retention, laws | `docs/compliance/COMPLIANCE.md`, `docs/compliance/DATA_CATALOG.md` |
| Security controls | `docs/SECURITY.md` |
| Media compression | `docs/MEDIA.md` |
| Hosting/cost/deploy | `docs/COST.md`, `docs/DEPLOYMENT.md` |
| Why things are the way they are | `docs/ARCHITECTURE.md#decisions` |

## Commands (Taskfile — works on Windows/WSL/Linux)
`task infra:up` (Postgres+Mailpit) · `task api` · `task web` · `task test` · `task test:api` (e2e, needs DB) · `task lint` · `task db:pii-report`

## Non-negotiable rules
- IMPORTANT: every new column holding personal data gets `COMMENT ON COLUMN … IS 'pii=… basis=… retention=…'` in its migration, is added to `users.export` (backend/internal/users/users.go) and to `docs/compliance/DATA_CATALOG.md`.
- Migrations are forward-only: new file `backend/migrations/NNNN_name.sql`; never edit a shipped one.
- SQL: parameters only (`$1`), never string-concatenate user input. Lists use `listquery` (whitelisted fields).
- Every query on user-owned rows filters by `owner_id = httpx.UserID(c)`.
- Errors: return `httpx.Err(status, "snake_code", "Human message")`; clients switch on `code`.
- Crypto only via `internal/platform/secure`. Secrets only via env (`backend/.env.example` documents all).
- Never log bodies, tokens, emails, phones. Side effects (email/SMS/media) go through `jobs.Enqueue`.
- Flutter: Riverpod `Notifier`/`AsyncNotifier` only (no legacy providers); colours/sizes from `AppTheme`/`Tokens`; new screens are `deferred as` in `app/router.dart`; `env/*.env` is PUBLIC.
- Lists in Flutter extend `PagedNotifier`; writes that must survive offline go through `Outbox`.
- After changes run the narrowest check: `go test ./internal/<pkg>/...` or `flutter test test/<file>`; then `task lint` before finishing.
- Update the relevant doc in the same change when behaviour changes (docs are the AI's memory).

## Token discipline (for Claude)
Prefer `grep`/targeted reads over reading whole dirs. Don't open `frontend/build`, `.dart_tool`, `*.lock`, `go.sum`, `lib/l10n/gen`.
When compacting, keep: files changed, commands that verify them, open TODOs.
