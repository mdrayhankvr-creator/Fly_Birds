-- "Forgot password": a time-limited, single-use reset link sent by email.
--
-- One table is both the token store and the abuse log, because the rate
-- limit has to count attempts for addresses that have no account as well.
-- A row for an unknown address therefore has no user and no token; it exists
-- only so the next attempt from that address or that client is counted.
--
-- The address is kept as a SHA-256 digest, so the table cannot be mined for
-- the school's email addresses, and the link itself is never stored: only
-- the SHA-256 of its random token, the same way refresh tokens are kept.
-- Setting a password through a link marks its row used, so the link stops
-- working, and the handler revokes the account's other outstanding links in
-- the same transaction.
--
-- Rows are pruned a week after they were made (auth.ResetRequestRetention):
-- the rate limit only looks an hour back and the lasting record of a reset is
-- the audit event, so nothing needs them longer. There is no index on
-- created_at for that: the limits keep the table small enough to scan.

CREATE TABLE password_reset_requests (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email_sha256 bytea NOT NULL CHECK (octet_length(email_sha256) = 32),
    requester_ip text NOT NULL DEFAULT '',
    user_id uuid REFERENCES users(id) ON DELETE CASCADE,
    token_sha256 bytea CHECK (token_sha256 IS NULL OR octet_length(token_sha256) = 32),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz,
    used_at timestamptz,
    -- An attempt for an address with no account (or one that may not use the
    -- link, such as a student sign-in without a real mailbox) is logged
    -- without a token, so it still counts toward the rate limit.
    CONSTRAINT password_reset_requests_token_ck CHECK ((token_sha256 IS NULL) = (user_id IS NULL)),
    CONSTRAINT password_reset_requests_expiry_ck CHECK ((expires_at IS NULL) = (token_sha256 IS NULL)),
    CONSTRAINT password_reset_requests_used_ck CHECK (used_at IS NULL OR token_sha256 IS NOT NULL)
);

CREATE UNIQUE INDEX password_reset_requests_token_key ON password_reset_requests (token_sha256) WHERE token_sha256 IS NOT NULL;
CREATE INDEX password_reset_requests_email_idx ON password_reset_requests (email_sha256, created_at DESC);
CREATE INDEX password_reset_requests_ip_idx ON password_reset_requests (requester_ip, created_at DESC) WHERE requester_ip <> '';
CREATE INDEX password_reset_requests_live_idx ON password_reset_requests (user_id) WHERE used_at IS NULL AND token_sha256 IS NOT NULL;
