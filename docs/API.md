# API reference (v1)

Base: `/v1`. JSON only. Auth: `Authorization: Bearer <access_token>`.
Mobile clients send `X-Client: mobile` to receive `refresh_token` in bodies; web gets it as an HttpOnly cookie.

## Conventions
- **Errors**: non-2xx → `{"error":{"code":"snake_case","message":"Human text","fields":{"email":"email"}}}`.
  Codes are stable API; messages are not. 422 = validation, 409 = conflict (body also has `current`).
- **Idempotency**: send `Idempotency-Key: <uuid>` on POST/PATCH/PUT/DELETE to make retries safe (24h window).
  Replays return the original response with header `Idempotent-Replay: true`.
- **Lists**: `?q=&filter[field]=v&filter[field][op]=v&sort=-field&limit=20&cursor=…`
  ops: `eq ne in gt gte lt lte` (`in` = comma list). `sort=relevance` when `q` is set.
  Response: `{"items":[…],"next_cursor":"…","has_more":true}`. Only whitelisted fields per endpoint.
- **Rate limits**: 300/min/IP global; 10/min/IP on auth endpoints; export 3/hour/user. 429 → `rate_limited`.
- **Timestamps**: RFC 3339 UTC. **IDs**: UUIDv7.

## Endpoints
| Method | Path | Auth | Notes |
|---|---|---|---|
| GET | /healthz, /readyz | – | liveness / DB readiness |
| GET | /v1/app/config | – | min_version, latest_version, maintenance, sms_login, policy versions, features |
| POST | /v1/auth/register | – | {email,password≥10,name?,locale?,accept_terms:true,marketing_consent?} → 202, emails code |
| POST | /v1/auth/verify-email | – | {email,code} → tokens |
| POST | /v1/auth/login | – | {email,password} → tokens · 401 invalid_credentials · 403 email_not_verified · 429 account_locked |
| POST | /v1/auth/otp/request | – | {channel:email\|sms, destination} → 202 (always; no enumeration) |
| POST | /v1/auth/otp/verify | – | {channel,destination,code} → tokens (creates account if SIGNUP_VIA_OTP) |
| POST | /v1/auth/password/forgot | – | {email} → 202 |
| POST | /v1/auth/password/reset | – | {email,code,new_password} → 204, revokes all sessions |
| POST | /v1/auth/refresh | cookie/body | rotates refresh token · 409 refresh_race (retry once) · 401 session_expired |
| POST | /v1/auth/logout | ✓ | revokes current session |
| GET/PATCH | /v1/me | ✓ | profile (phone masked) / {name?,locale?} |
| GET/PUT | /v1/me/consents | ✓ | current map / {purpose,granted} (appends to ledger) |
| GET | /v1/me/export | ✓ | JSON download of all personal data (GDPR Art. 15/20) |
| DELETE | /v1/me | ✓ | {confirm:"DELETE"} → 202 {purge_at}; sign-in during grace cancels |
| GET/DELETE | /v1/me/sessions[/:id] | ✓ | list / revoke all / revoke one |
| GET/POST | /v1/notes | ✓ | list (filters: archived, tags, created_at · sorts: updated_at, created_at, title · q) / create |
| GET/PATCH/DELETE | /v1/notes/:id | ✓ | PATCH requires `version` → 409 with `current` on conflict |
| GET | /v1/media | ✓ | list (filters: kind, status) |
| POST | /v1/media/uploads | ✓ | {filename,content_type,size,visibility?} → {id,upload_url,method,headers} |
| POST | /v1/media/:id/complete | ✓ | after PUT to upload_url → 202 processing |
| GET/DELETE | /v1/media/:id | ✓ | metadata + short-lived `url` and `variants` (thumb/md/poster…) |

Token response: `{"access_token","expires_at","refresh_token"?, "user":{"id","email","name","role"}}`.
