-- Core platform tables: identity, sessions, OTP, consent, audit, jobs, media.
-- Requires PostgreSQL 18+ (uuidv7(): time-ordered ids => compact, index-friendly).
-- Convention: columns ending in _enc hold AES-GCM ciphertext (see internal/security),
-- columns ending in _bidx hold HMAC blind indexes used for equality lookups.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE users (
    id                uuid PRIMARY KEY DEFAULT uuidv7(),
    email_enc         bytea NOT NULL,
    email_bidx        bytea NOT NULL UNIQUE,
    name_enc          bytea,
    phone_enc         bytea,
    phone_bidx        bytea UNIQUE,
    phone_verified_at timestamptz,
    password_hash     text,
    role              text NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    status            text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'locked')),
    locale            text NOT NULL DEFAULT 'en',
    email_verified_at timestamptz,
    last_login_at     timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
COMMENT ON COLUMN users.email_enc IS 'PII (contact). AES-256-GCM, aad=users.email';
COMMENT ON COLUMN users.name_enc  IS 'PII (identity). AES-256-GCM, aad=users.name';
COMMENT ON COLUMN users.phone_enc IS 'PII (contact). AES-256-GCM, aad=users.phone';

CREATE TABLE sessions (
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    family_id    uuid NOT NULL,              -- all rotations of one login share a family
    token_hash   bytea NOT NULL UNIQUE,      -- sha256 of the refresh token
    user_agent   text NOT NULL DEFAULT '',   -- truncated to 200 chars
    ip_trunc     text NOT NULL DEFAULT '',   -- /24 (v4) or /48 (v6) – pseudonymised
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL,
    rotated_at   timestamptz,                -- set when replaced by a newer token
    revoked_at   timestamptz
);
CREATE INDEX sessions_user ON sessions (user_id);
CREATE INDEX sessions_family ON sessions (family_id);
CREATE INDEX sessions_expires ON sessions (expires_at);

CREATE TABLE otp_codes (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    purpose     text NOT NULL,               -- login | verify_phone
    target_bidx bytea NOT NULL,              -- blind index of the email/phone
    target_enc  bytea NOT NULL,              -- encrypted email/phone (needed to finish sign-up)
    code_hash   bytea NOT NULL,
    link_hash   bytea UNIQUE,                -- magic-link token hash
    attempts    int NOT NULL DEFAULT 0,
    expires_at  timestamptz NOT NULL,
    used_at     timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX otp_target ON otp_codes (target_bidx, purpose, created_at DESC);
CREATE INDEX otp_expires ON otp_codes (expires_at);

-- Proof of consent (GDPR Art. 7(1)): who agreed to which version, when. Append-only.
CREATE TABLE consents (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    purpose    text NOT NULL,                -- terms | privacy | marketing_email | analytics …
    version    text NOT NULL,                -- version/date of the document agreed to
    granted    boolean NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX consents_user ON consents (user_id, purpose, created_at DESC);

-- Security audit trail. actor_id is nulled on account deletion (pseudonymised),
-- rows are purged after RETENTION_AUDIT_DAYS.
CREATE TABLE audit_log (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_id   uuid REFERENCES users (id) ON DELETE SET NULL,
    action     text NOT NULL,                -- auth.login, account.export, item.delete …
    target     text NOT NULL DEFAULT '',
    meta       jsonb NOT NULL DEFAULT '{}',  -- never put PII here
    ip_trunc   text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_actor ON audit_log (actor_id, created_at DESC);
CREATE INDEX audit_created ON audit_log (created_at);

-- Background jobs (Postgres queue: FOR UPDATE SKIP LOCKED). No Redis needed.
CREATE TABLE jobs (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind         text NOT NULL,
    payload      bytea NOT NULL,             -- encrypted JSON (may contain PII)
    run_at       timestamptz NOT NULL DEFAULT now(),
    attempts     int NOT NULL DEFAULT 0,
    max_attempts int NOT NULL DEFAULT 8,
    locked_until timestamptz,
    last_error   text,
    done_at      timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX jobs_ready ON jobs (run_at) WHERE done_at IS NULL;
CREATE INDEX jobs_done ON jobs (done_at) WHERE done_at IS NOT NULL;

CREATE TABLE media (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    owner_id      uuid REFERENCES users (id) ON DELETE CASCADE,
    sha256        bytea NOT NULL,            -- of the original: per-owner de-duplication
    kind          text NOT NULL,             -- image | video | audio | pdf | archive | document | text | other
    mime          text NOT NULL,
    name_enc      bytea,                     -- original filename (can contain personal data)
    visibility    text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private', 'public')),
    status        text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'ready', 'failed')),
    size_original bigint NOT NULL,
    size_stored   bigint NOT NULL DEFAULT 0,
    width         int,
    height        int,
    duration_s    real,
    variants      jsonb NOT NULL DEFAULT '[]', -- [{name,key,mime,encoding,width,height,size}]
    error         text,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX media_owner_sha ON media (owner_id, sha256);
CREATE INDEX media_owner_created ON media (owner_id, created_at DESC);
