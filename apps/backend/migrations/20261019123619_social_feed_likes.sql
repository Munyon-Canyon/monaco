CREATE TABLE feed_likes (
  feed_object_id uuid NOT NULL REFERENCES feed_objects (id),
  user_id uuid NOT NULL REFERENCES users (id),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (feed_object_id, user_id)
);

CREATE INDEX feed_likes_user_idx ON feed_likes (user_id);
