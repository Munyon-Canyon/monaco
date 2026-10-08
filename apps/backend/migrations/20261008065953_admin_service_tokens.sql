CREATE TABLE admin_service_tokens (
  id uuid PRIMARY KEY,
  name text NOT NULL CHECK (name ~ '^[a-z0-9][a-z0-9_-]{0,62}$'),
  token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
  created_at timestamptz NOT NULL,
  created_by text NOT NULL,
  revoked_at timestamptz
);

CREATE UNIQUE INDEX admin_service_tokens_live_name ON admin_service_tokens (name) WHERE revoked_at IS NULL;
