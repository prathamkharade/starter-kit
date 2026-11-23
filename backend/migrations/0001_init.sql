-- 0001_init: core platform tables + the `notes` reference module.
-- Requires PostgreSQL 18+ (uuidv7()). On older versions replace uuidv7() with gen_random_uuid().
--
-- PII TAGGING CONVENTION (machine-readable data map, see docs/compliance/DATA_CATALOG.md):
--   COMMENT ON COLUMN t.c IS 'pii=<category> basis=<lawful basis> retention=<rule>';
--   `task db:pii-report` lists every tagged column. Tag every new personal-data column.

CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN NEW.updated_at = now(); RETURN NEW; END $$ LANGUAGE plpgsql;

-- ---------------------------------------------------------------- users
CREATE TABLE users (
  id                uuid PRIMARY KEY DEFAULT uuidv7(),
  email             citext UNIQUE,
  email_verified_at timestamptz,
  phone_enc         bytea,           -- AES-256-GCM ciphertext (secure.Keyring)
  phone_hash        bytea UNIQUE,    -- HMAC blind index for lookups
  phone_verified_at timestamptz,
  password_hash     text,            -- argon2id PHC string; NULL for passwordless users
  name              text NOT NULL DEFAULT '',
  locale            text NOT NULL DEFAULT 'en',
  role              text NOT NULL DEFAULT 'user' CHECK (role IN ('user','admin')),
  status            text NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended','deleted')),
  terms_version     text,
  terms_accepted_at timestamptz,
  failed_logins     int  NOT NULL DEFAULT 0,
  locked_until      timestamptz,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now(),
  deleted_at        timestamptz,     -- set on erasure request; purged after grace period
  CHECK (email IS NOT NULL OR phone_hash IS NOT NULL)
);
CREATE TRIGGER users_updated BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION set_updated_at();
COMMENT ON COLUMN users.email        IS 'pii=contact basis=contract retention=account+grace';
COMMENT ON COLUMN users.phone_enc    IS 'pii=contact basis=contract retention=account+grace encrypted=aes-gcm';
COMMENT ON COLUMN users.phone_hash   IS 'pii=pseudonymous basis=contract retention=account+grace';
COMMENT ON COLUMN users.name         IS 'pii=identity basis=contract retention=account+grace';
COMMENT ON COLUMN users.password_hash IS 'pii=credential basis=contract retention=account';

-- ---------------------------------------------------------------- sessions (refresh tokens)
CREATE TABLE sessions (
  id           uuid PRIMARY KEY DEFAULT uuidv7(),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  family_id    uuid NOT NULL,          -- rotation chain; reuse of a rotated token revokes the family
  token_hash   bytea NOT NULL UNIQUE,  -- sha256(refresh token); raw token never stored
  user_agent   text NOT NULL DEFAULT '',
  ip_hash      bytea,
  created_at   timestamptz NOT NULL DEFAULT now(),
  last_used_at timestamptz NOT NULL DEFAULT now(),
  expires_at   timestamptz NOT NULL,
  revoked_at   timestamptz
);
CREATE INDEX sessions_user ON sessions(user_id);
CREATE INDEX sessions_family ON sessions(family_id);
COMMENT ON COLUMN sessions.user_agent IS 'pii=device basis=legitimate_interest(security) retention=session+30d';
COMMENT ON COLUMN sessions.ip_hash    IS 'pii=pseudonymous basis=legitimate_interest(security) retention=session+30d';

-- ---------------------------------------------------------------- one-time codes
CREATE TABLE otp_codes (
  id               uuid PRIMARY KEY DEFAULT uuidv7(),
  purpose          text NOT NULL,       -- verify_email | login | reset_password | verify_phone
  destination_hash bytea NOT NULL,      -- HMAC(email|phone); destination itself not stored
  code_hash        bytea NOT NULL,
  attempts         int NOT NULL DEFAULT 0,
  expires_at       timestamptz NOT NULL,
  consumed_at      timestamptz,
  created_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX otp_lookup ON otp_codes(destination_hash, purpose, created_at DESC);

-- ---------------------------------------------------------------- consent ledger (append-only)
CREATE TABLE consents (
  id             uuid PRIMARY KEY DEFAULT uuidv7(),
  user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  purpose        text NOT NULL,         -- terms | privacy | marketing | analytics | ...
  granted        boolean NOT NULL,
  policy_version text NOT NULL,
  source         text NOT NULL DEFAULT 'app',
  created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX consents_user ON consents(user_id, purpose, created_at DESC);
COMMENT ON TABLE consents IS 'GDPR Art.7(1) proof of consent. Never UPDATE; INSERT a new row to change.';

-- ---------------------------------------------------------------- audit log (security events)
CREATE TABLE audit_log (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id    uuid,                      -- no FK: survives erasure as a pseudonymous id
  action     text NOT NULL,
  ip_hash    bytea,
  meta       jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_user ON audit_log(user_id, created_at DESC);
CREATE INDEX audit_time ON audit_log(created_at);
COMMENT ON COLUMN audit_log.ip_hash IS 'pii=pseudonymous basis=legitimate_interest(security) retention=AUDIT_RETENTION';

-- ---------------------------------------------------------------- background jobs
CREATE TABLE jobs (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  kind         text NOT NULL,
  payload      jsonb NOT NULL DEFAULT '{}',
  status       text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','done','failed')),
  attempts     int NOT NULL DEFAULT 0,
  max_attempts int NOT NULL DEFAULT 5,
  run_at       timestamptz NOT NULL DEFAULT now(),
  locked_at    timestamptz,
  last_error   text,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX jobs_ready ON jobs(run_at, id) WHERE status = 'queued';
COMMENT ON COLUMN jobs.payload IS 'pii=transient basis=contract retention=7d (cleanup job)';

-- ---------------------------------------------------------------- idempotency (offline replay safety)
CREATE TABLE idempotency_keys (
  user_id     uuid NOT NULL,
  key         text NOT NULL,
  method      text NOT NULL,
  path        text NOT NULL,
  status_code int,
  response    bytea,
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, key)
);

-- ---------------------------------------------------------------- media
CREATE TABLE media (
  id               uuid PRIMARY KEY DEFAULT uuidv7(),
  owner_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind             text NOT NULL CHECK (kind IN ('image','video','audio','document','archive','other')),
  status           text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','ready','failed')),
  visibility       text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','public')),
  original_name    text NOT NULL,
  content_type     text NOT NULL,
  size_bytes       bigint NOT NULL DEFAULT 0,
  stored_bytes     bigint NOT NULL DEFAULT 0,
  sha256           bytea,
  storage_key      text NOT NULL,       -- key of the primary (compressed) object
  content_encoding text,                -- e.g. gzip for generic compressible files
  variants         jsonb NOT NULL DEFAULT '{}',  -- {"thumb":{"key":..,"w":..,"bytes":..}, ...}
  error            text,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER media_updated BEFORE UPDATE ON media FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE INDEX media_owner ON media(owner_id, created_at DESC, id DESC);
CREATE INDEX media_dedupe ON media(owner_id, sha256) WHERE sha256 IS NOT NULL;
COMMENT ON COLUMN media.original_name IS 'pii=possible basis=contract retention=account';

-- ---------------------------------------------------------------- notes (REFERENCE MODULE — copy for new entities)
CREATE TABLE notes (
  id         uuid PRIMARY KEY DEFAULT uuidv7(),
  owner_id   uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  title      text NOT NULL,
  body       text NOT NULL DEFAULT '',
  tags       text[] NOT NULL DEFAULT '{}',
  archived   boolean NOT NULL DEFAULT false,
  version    int NOT NULL DEFAULT 1,     -- optimistic concurrency for offline sync
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  search     tsvector GENERATED ALWAYS AS (
               setweight(to_tsvector('simple', title), 'A') ||
               setweight(to_tsvector('simple', body),  'B')) STORED
);
CREATE TRIGGER notes_updated BEFORE UPDATE ON notes FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE INDEX notes_owner_updated ON notes(owner_id, updated_at DESC, id DESC);
CREATE INDEX notes_owner_created ON notes(owner_id, created_at DESC, id DESC);
CREATE INDEX notes_search ON notes USING gin(search);
CREATE INDEX notes_title_trgm ON notes USING gin(title gin_trgm_ops);
CREATE INDEX notes_tags ON notes USING gin(tags);
COMMENT ON COLUMN notes.body IS 'pii=user_content basis=contract retention=account';
