CREATE TABLE catch_up_state (
  user_id uuid NOT NULL REFERENCES users (id),
  item_key text NOT NULL,
  read_at timestamptz,
  cleared_at timestamptz,
  cleared_reason text CHECK (cleared_reason IN ('voted', 'replied', 'joined', 'cleared')),
  kept_at timestamptz,
  snapshot jsonb,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (user_id, item_key)
);

ALTER TABLE notifications ADD COLUMN item_key text;

CREATE INDEX notifications_user_id_item_key_created_at_idx ON notifications (user_id, item_key, created_at DESC);

ALTER TABLE notifications
  DROP CONSTRAINT notifications_state_check,
  ADD CONSTRAINT notifications_state_check CHECK (state IN ('pending', 'delivered', 'no_device', 'batched', 'muted'));
