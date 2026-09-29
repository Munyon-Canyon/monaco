CREATE TABLE idempotency_keys (
  actor_key text NOT NULL,
  key text NOT NULL,
  request_hash bytea NOT NULL,
  status smallint NOT NULL CHECK (status IN (1, 2)),
  response_status int,
  response_body bytea,
  response_headers jsonb,
  created_at timestamptz NOT NULL,
  completed_at timestamptz,
  PRIMARY KEY (actor_key, key)
);

CREATE INDEX idempotency_keys_created_at_idx ON idempotency_keys (created_at);
