# Security

"Hacking-proof" doesn't exist; the goal is defence in depth so that one mistake isn't a breach,
and when something happens you can detect, contain and report it within legal deadlines.
Controls map to OWASP ASVS 5.0 / OWASP Top 10.

## Authentication & sessions
- Passwords: argon2id (m=19 MiB, t=2, p=1), min 10 chars, max 128; timing-equalised unknown-user path.
- Lockout: 5 failures → 15 min lock (per account) + 10/min/IP on auth routes.
- OTP: 6 digits from CSPRNG, stored as HMAC, 10 min, 5 attempts, single-use (atomic consume), send throttling.
- No account enumeration: register/OTP/forgot respond identically for unknown accounts.
- Access token: JWT HS256 pinned (rejects `alg=none`/confusion), iss/aud/exp checked, 15 min.
- Refresh token: 256-bit random, stored as SHA-256, rotated on every use; reuse of a rotated token revokes the
  whole family (theft detection) and is audited. Web: HttpOnly + Secure + SameSite=Strict cookie scoped to `/v1/auth`.
- Password reset/“sign out everywhere” revoke all sessions.
- Add later if needed: TOTP/passkeys (WebAuthn) for admins, breached-password check (HIBP k-anonymity).

## Authorization
Every user-owned query filters by `owner_id`; others' rows return 404. Roles via `RequireRole`. Admin
tooling should be a separate, IP-restricted surface.

## Input & output
Struct validation on every body; JSON body limit 1 MiB; parameterised SQL only; list fields whitelisted;
uploads go to storage with random keys, type sniffed from bytes, size checked after upload, non-media served
as `attachment`, SVG treated as document; ffmpeg restricted (`-protocol_whitelist file`, demuxer whitelist).

## Transport & headers
TLS by Caddy (auto-renew), HSTS preload, HTTP/3, CSP, X-Content-Type-Options, Referrer-Policy,
Permissions-Policy, COOP, frame-ancestors none. Cloudflare in front: hide origin IP, WAF, DDoS.

## Data protection
Field encryption (AES-256-GCM) for high-risk identifiers (phone example) with HMAC blind indexes; IPs stored
only as truncated HMAC; disk encryption at provider; backups encrypted with `age` (private key offline);
logs exclude bodies/headers/query strings and use route patterns (no IDs).

## Infrastructure
Non-root containers, distroless API, read-only root FS, `no-new-privileges`, all capabilities dropped,
memory/CPU limits, Postgres not exposed, SSH keys only, ufw, fail2ban, unattended upgrades, swap.
Secrets only in `infra/.env.prod` (600) on the server; prod refuses dev secrets and console email.

## Supply chain
`go.sum`/`pubspec.lock` committed, Dependabot weekly, `govulncheck` + gitleaks in CI, pinned base images
(update monthly), builds in CI not on laptops.

## Audit events (`audit_log.action`)
user.registered · auth.login_succeeded · auth.login_failed · auth.account_locked · auth.refresh_reuse_detected ·
auth.password_reset · auth.logout_all · privacy.consent_changed · privacy.data_exported ·
privacy.deletion_requested · privacy.deletion_cancelled · privacy.user_purged

## Incident response (keep this current)
1. Contain: rotate `JWT_SECRET` (logs everyone out), revoke sessions (`UPDATE sessions SET revoked_at=now()`),
   block IPs at Cloudflare, take a DB snapshot for forensics.
2. Assess with `audit_log` + Caddy/app logs: what data, whose, since when.
3. Notify — clocks start when you become aware:
   - **GDPR Art. 33**: supervisory authority within **72 h** if risk to people; Art. 34: tell users if high risk.
   - **EU Cyber Resilience Act** (reporting applies since 11 Sep 2026 to products with digital elements, incl.
     apps): actively exploited vulnerability / severe incident → early warning to CSIRT/ENISA platform within
     **24 h**, notification within 72 h, final report later.
   - **NIS2** (if your org/sector is in scope): 24 h early warning, 72 h notification.
   - Others by market: India CERT-In 6 h; US state breach laws; sector rules (HIPAA, PCI).
4. Fix root cause, add a test, post-mortem in `docs/incidents/`.
Publish a `/.well-known/security.txt` and a vulnerability disclosure policy (CRA expects coordinated disclosure).
