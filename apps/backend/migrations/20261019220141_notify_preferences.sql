CREATE TABLE notification_preferences (
  user_id uuid NOT NULL REFERENCES users (id),
  category text NOT NULL CHECK (category IN ('decision_requests', 'replies_and_mentions')),
  enabled boolean NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (user_id, category)
);
