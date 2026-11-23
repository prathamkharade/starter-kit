# App Starter Kit — Flutter (web + mobile) · Go · PostgreSQL

Put it in git, describe your idea to Claude Code, build. Everything common to most apps is done and
replaceable: auth with email/SMS codes, privacy & compliance plumbing, search/filter/pagination, offline sync,
compressed media storage, adaptive UI, tests, CI/CD, and a €6–12/month production setup.

**Start here → [`docs/GETTING_STARTED.md`](docs/GETTING_STARTED.md)** (Windows + VS Code + Claude Code, step by step).

```
backend/     Go 1.26 · Fiber v3 · pgx v5 · golang-jwt · godotenv · x/crypto (argon2id, AES-GCM)
frontend/    Flutter · riverpod 3 · go_router · dio · flutter_dotenv · form_builder · permission_handler · hive_ce
infra/       compose (dev + prod) · Caddy · Dockerfiles · server/deploy/backup scripts · k6
docs/        architecture, API, security, compliance, cost, deployment… (AI-readable, one topic per file)
.claude/     skills (/new-app, /add-entity, /security-review, /ship-check) + permission settings
CLAUDE.md    short index the AI reads first
Taskfile.yml one command set for Windows, WSL, Linux and CI
```

## How each requirement is met
| # | Requirement | Where |
|---|---|---|
| 1 | Strict (EU-first) compliance; what data to store and how | `docs/compliance/` (law matrix, data catalog by app type & stakeholder, storage tiers); consent ledger, export, erasure w/ grace, retention jobs, PII-tagged schema (`task db:pii-report`) |
| 2 | Intuitive, changeable UI/UX | one theme file + tokens, adaptive nav (bar/rail), reusable list/search/empty/error widgets, dark mode, i18n, accessibility defaults — `docs/FRONTEND.md` |
| 3 | Cheap email/OTP/SMS, just add details | SMTP (any provider) + Twilio/generic HTTP SMS via env; queued with retries; OTP abuse limits — `docs/BACKEND.md` (Notifications) |
| 4 | Fast web load | CSS splash, deferred screens, Wasm, self-hosted engine, brotli, HTTP/3, CDN, service worker |
| 5, 13 | Cost-effective hosting | single VPS + Cloudflare free + R2 (no egress), one Go binary, Postgres does queue+search — `docs/COST.md` |
| 6 | Offline | cached reads, persistent outbox + idempotency keys, version conflicts (409), offline banner |
| 7 | Search/filter/pagination | backend `listquery` (whitelisted, keyset cursors, FTS + fuzzy) ↔ Flutter `ListQuery`/`PagedNotifier`/`PagedListView` |
| 8 | Any media, heavily compressed | presigned direct upload; WebP renditions, H.264/HEVC/AV1, AAC, Ghostscript, gzip; EXIF stripped — `docs/MEDIA.md` |
| 9 | Extra essentials | remote config (force update, maintenance, flags), sessions mgmt, audit log, health checks, i18n, security headers, backups |
| 10–12 | Required stacks | see top of this file; Postgres 18 |
| 14 | Minimal tokens with Claude Code | short CLAUDE.md router, topic docs, reference module, skills, read-deny rules — `docs/GETTING_STARTED.md#part-d` |
| 15 | AI-ready documentation | `CLAUDE.md` + `docs/*` + decision log in `docs/ARCHITECTURE.md` |
| 16 | Scalable, hardened | stateless API, scaling ladder, defence in depth — `docs/SECURITY.md` |
| 17 | Docker without Docker Desktop | WSL2 + Docker Engine, `.wslconfig` limits, same images in prod |
| 18 | Testing ready | Go unit + e2e API, Flutter unit/widget/integration, k6, lint/vuln/secret scans in CI — `docs/TESTING.md` |

## Quick start
```bash
task setup && task infra:up
task api     # terminal 1
task web     # terminal 2 → sign in, code appears at http://localhost:8025
```

## Status of this kit
Backend: type-checked against the real dependency sources, unit tests pass, all SQL validated against
PostgreSQL. Frontend: `flutter analyze` clean and `flutter build web --wasm` succeeds on Flutter 3.50. UI imports use
`package:material_ui` / `package:cupertino_ui` (Material/Cupertino left the SDK in Flutter 3.47; mixing in
`package:flutter/material.dart` breaks `flutter_form_builder` 11). On Windows, `flutter test` needs the
Visual Studio "Desktop development with C++" workload (native build hook of `hive_ce_flutter`). See `docs/TESTING.md`.

License: yours. Not legal advice — see `docs/compliance/COMPLIANCE.md`.
