CREATE TABLE money_intents (
  actor_key text NOT NULL,
  key text NOT NULL,
  operation text NOT NULL,
  request_hash bytea NOT NULL,
  resource_id uuid NOT NULL,
  created_at timestamptz NOT NULL,
  PRIMARY KEY (actor_key, key),
  UNIQUE (operation, resource_id)
);
