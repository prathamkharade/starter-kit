# Architecture

## Shape
```
 Browser / Android / iOS  (one Flutter codebase)
        │  HTTPS (HTTP/3), JSON, Bearer access token (15 min) + refresh token (cookie on web / Keychain on mobile)
        ▼
 Cloudflare (free: DNS, CDN, DDoS, WAF)  ──►  VPS
                                              ├─ caddy   : TLS, brotli, security headers, serves Flutter web, /v1 → api
                                              ├─ api     : Go + Fiber v3 (stateless)
                                              ├─ worker  : same binary, `worker` mode: email/SMS/media/retention jobs
                                              └─ db      : PostgreSQL 18 (data + job queue + full-text search)
        Files: client ──presigned PUT/GET──► S3-compatible storage (Cloudflare R2: no egress fees)
```
One binary, one database, no Redis/Elastic/RabbitMQ. Postgres does queueing (`SKIP LOCKED`), search
(`tsvector` + `pg_trgm`), and idempotency. Fewer moving parts = cheaper and less to maintain.

## Backend layout (`backend/`)
```
cmd/app/main.go            serve | worker | migrate | secrets | purge-user
migrations/*.sql           forward-only, embedded, run at startup (advisory-locked)
internal/config            env → Config (documented in .env.example)
internal/server            ALL routes + middleware + job registrations (start here)
internal/platform/
  httpx                    error envelope, Bind/Validate, Auth, RequestLogger, Idempotency
  listquery                search/filter/sort/cursor pagination → safe SQL
  jobs                     Postgres queue, retries/backoff, periodic tasks
  secure                   argon2id, AES-GCM field encryption, HMAC blind index, random
  token                    JWT access tokens (HS256 pinned)
  notify                   email (console/SMTP) + SMS (twilio/http) + templates
  storage                  S3 presign/upload/delete
  audit                    security event log
  kit                      Deps container + shared helpers (modules depend on kit, never on each other)
internal/auth|users|media|notes   feature modules (notes = reference to copy)
```

## Frontend layout (`frontend/lib/`)
```
main.dart, bootstrap.dart  env + cache init, error hooks
app/                       app.dart, router.dart (routes, tabs, auth redirect), theme/, preferences.dart
core/                      config/env, network (dio + refresh), offline (connectivity, outbox),
                           pagination (ListQuery, PagedNotifier), storage, permissions, utils
shared/widgets/            AppShell (adaptive nav), PagedListView, SearchFilterBar, Empty/Error states
features/<name>/           data (repository) → application (Notifier) → presentation (pages)
l10n/                      app_en.arb, app_de.arb → generated AppLocalizations
```

## Request lifecycle
recover → request id → logger (no PII) → helmet → CORS → global rate limit (300/min/IP)
→ route group: `Auth` (JWT) → `Idempotency` (unsafe methods with `Idempotency-Key`) → handler
→ errors converted to `{"error":{code,message,fields}}` by `httpx.ErrorHandler`.

## Scaling ladder (do the next step only when metrics say so)
1. **1 VPS** (2 vCPU / 4 GB): thousands of DAU. Cloudflare caches the web app.
2. **Bigger VPS** (vertical) — cheapest real step. Tune Postgres `shared_buffers`.
3. **Split worker** to its own small VPS (video encoding off the API box). Same images.
4. **Managed Postgres** or a dedicated DB box + PgBouncer; nightly backups → PITR (WAL-G).
5. **N API replicas** behind a load balancer. API is stateless; move rate-limit storage to Postgres/Redis
   (`limiter.Config.Storage`, e.g. `gofiber/storage/postgres`). Read replicas for heavy lists.
6. Partition big tables (audit_log, events) by month; move search to Meilisearch/OpenSearch only if
   Postgres FTS becomes the bottleneck.

## Decisions
| # | Decision | Why | Revisit when |
|---|---|---|---|
| 1 | Postgres for queue/search/idempotency | one system to run/back up; good to ~millions of rows | queue > ~1k jobs/s |
| 2 | JWT access (15m) + rotating opaque refresh (DB) | stateless API, revocable sessions, theft detection | need instant revocation of access tokens |
| 3 | Refresh token in HttpOnly cookie on web | not readable by XSS | never |
| 4 | Presigned direct uploads | zero file bandwidth through API; cheaper, faster | need server-side streaming transforms |
| 5 | WebP default (not AVIF) | Flutter mobile can't decode AVIF natively | Flutter adds AVIF |
| 6 | H.264 CRF 28 ≤720p default | universal playback, cheap CPU | storage costs dominate → AV1 |
| 7 | Field-level encryption for phone (example) | breach containment for high-risk identifiers | encrypt more fields per DPIA |
| 8 | Email kept plaintext (citext) | needed for login/uniqueness/support; protected by disk encryption + access control | DPIA says otherwise |
| 9 | uuidv7 primary keys | time-ordered (index-friendly), non-guessable, merge-safe offline | — |
| 10 | Hand-written SQL + pgx (no ORM) | transparent, fast, AI-readable | team prefers sqlc → easy to add |
| 11 | Flutter deferred imports per screen | smaller first download on web | — |
| 12 | Caddy over nginx | automatic HTTPS, HTTP/3, tiny config | — |
| 13 | Hive CE for offline cache/outbox | pure Dart, IndexedDB on web, no sqlite wasm download | need relational offline queries → drift |
| 14 | RustFS for local S3 | MinIO community images withdrawn (2026) | — |
