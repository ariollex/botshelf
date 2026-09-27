CREATE TABLE IF NOT EXISTS bots (
    id TEXT PRIMARY KEY,
    username VARCHAR(60) NOT NULL UNIQUE,
    name VARCHAR(80) NOT NULL,
    description VARCHAR(500) NOT NULL,
    category VARCHAR(40) NOT NULL,
    source_url TEXT NOT NULL,
    last_verified_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS bots_category_idx ON bots (category);
CREATE INDEX IF NOT EXISTS bots_last_verified_at_idx ON bots (last_verified_at DESC);

CREATE TABLE IF NOT EXISTS submissions (
    id TEXT PRIMARY KEY,
    username VARCHAR(60) NOT NULL,
    name VARCHAR(80) NOT NULL,
    description VARCHAR(500) NOT NULL,
    category VARCHAR(40) NOT NULL,
    source_url TEXT NOT NULL,
    owner_contact VARCHAR(120) NOT NULL,
    status VARCHAR(16) NOT NULL CHECK (status IN ('pending', 'verified', 'rejected')),
    created_at TIMESTAMPTZ NOT NULL,
    reviewed_at TIMESTAMPTZ,
    reviewer_id BIGINT,
    moderator_note VARCHAR(500) NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS submissions_pending_username_idx
    ON submissions (username) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS submissions_pending_created_at_idx
    ON submissions (created_at ASC) WHERE status = 'pending';

CREATE TABLE IF NOT EXISTS moderator_sessions (
    token_hash BYTEA PRIMARY KEY,
    user_id BIGINT NOT NULL,
    first_name TEXT NOT NULL DEFAULT '',
    last_name TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS moderator_sessions_expires_at_idx ON moderator_sessions (expires_at);
