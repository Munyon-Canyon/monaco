CREATE TABLE device_tokens (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id),
  token text NOT NULL UNIQUE,
  environment text NOT NULL CHECK (environment IN ('sandbox', 'production')),
  created_at timestamptz NOT NULL,
  last_seen_at timestamptz NOT NULL,
  disabled_at timestamptz
);

CREATE INDEX device_tokens_active_user_id_idx ON device_tokens (user_id) WHERE disabled_at IS NULL;

CREATE TABLE notification_broadcasts (
  id uuid PRIMARY KEY,
  kind text NOT NULL,
  source_event_id uuid NOT NULL UNIQUE,
  recipient_count int NOT NULL,
  created_at timestamptz NOT NULL
);

CREATE TABLE notifications (
  id uuid PRIMARY KEY,
  broadcast_id uuid REFERENCES notification_broadcasts (id),
  user_id uuid NOT NULL REFERENCES users (id),
  kind text NOT NULL,
  source_event_id uuid NOT NULL,
  title text NOT NULL,
  body text NOT NULL,
  data jsonb NOT NULL,
  collapse_id text NOT NULL,
  state text NOT NULL CHECK (state IN ('pending', 'delivered', 'no_device', 'batched')),
  created_at timestamptz NOT NULL,
  delivered_at timestamptz,
  UNIQUE (source_event_id, user_id, kind)
);

CREATE INDEX notifications_user_id_kind_created_at_idx ON notifications (user_id, kind, created_at);
