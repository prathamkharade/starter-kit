# Cost engineering

Prices change — re-check before buying. Figures below were checked November 2026.

## Starting stack (≈ €6–12 / month)
| Item | Choice | Cost |
|---|---|---|
| Server | 1 VPS, 2 vCPU / 4 GB, EU region (Hetzner CX23 ≈ €5.49 + IPv4 when in stock; else Netcup, OVH, Contabo, Hetzner CAX ARM) | €5–10 |
| CDN/DNS/TLS/WAF | Cloudflare Free | €0 |
| Object storage | Cloudflare R2: 10 GB-month free, then $0.015/GB-month, **no egress fees** | €0–2 |
| Email | Brevo free (300/day) or Resend free (3,000/month) to start; Amazon SES at volume (~$0.10–0.16 per 1,000) | €0 → cents |
| SMS | Only where needed; email/WhatsApp codes first. Restrict countries | per message |
| Backups | R2/B2 bucket, compressed+encrypted dumps | < €1 |
| CI/CD | GitHub Actions (free minutes for public repos; private repos have a monthly allowance) + GHCR | €0 |
| Monitoring | UptimeRobot/Better Stack free uptime checks; `docker stats`; Sentry/GlitchTip free tier if needed | €0 |

Hetzner raised cloud prices twice in 2026 and its cheapest "cost-optimized" plans are often sold out; the stack is
provider-agnostic (any VPS with Docker), so pick whatever EU VPS gives ≥2 GB RAM cheapest. ARM VPSes need
`linux/arm64` images (one line in `.github/workflows/deploy.yml`).

## Why it stays cheap
- **One binary, one database.** No Redis, Elasticsearch, Kafka, Kubernetes, or managed services to pay for.
- **Go**: ~20–40 MB RAM idle API; thousands of req/s on 2 vCPUs.
- **Files never touch the server**: presigned uploads/downloads; R2 has zero egress (the usual cloud bill killer).
- **Compression everywhere**: media (60–90% smaller), brotli for the web app, WebP thumbnails → less storage *and* less bandwidth.
- **Cloudflare caches** the static Flutter web build at the edge.
- **Builds run in CI**, not on the 4 GB server.
- **Cursor pagination + indexes** keep DB CPU flat as data grows.
- **Retention jobs** delete what you don't need (compliance *and* storage).
- **Background jobs in Postgres** (no queue service).

## Cost traps to avoid
SMS OTP at scale (use email codes / WhatsApp / passkeys; enable SMS fraud protection), egress from
S3/GCS (use R2), managed databases early (€15–50+/month), serving video originals, logging everything forever,
over-provisioning "for scale" before metrics say so.

## When to spend more (in order)
More RAM on the VPS → separate worker VPS → managed/dedicated Postgres with PITR → multiple API nodes. See
`docs/ARCHITECTURE.md#scaling-ladder`.
