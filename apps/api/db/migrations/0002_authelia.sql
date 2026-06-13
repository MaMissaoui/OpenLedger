-- +goose Up
-- Authentication is now handled by Authelia + lldap. Users are provisioned
-- just-in-time on their first request via the Remote-User header set by Traefik.

-- Allow LDAP-authenticated users (no password stored in this DB).
ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;

-- Stable LDAP identifier for JIT provisioning — matches the lldap uid attribute.
ALTER TABLE users ADD COLUMN ldap_uid TEXT UNIQUE;

-- Authelia + Redis own session management; refresh tokens are no longer needed.
DROP TABLE refresh_tokens;

-- +goose Down
CREATE TABLE refresh_tokens (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);
CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(user_id);

ALTER TABLE users DROP COLUMN ldap_uid;
ALTER TABLE users ALTER COLUMN password_hash SET NOT NULL;
