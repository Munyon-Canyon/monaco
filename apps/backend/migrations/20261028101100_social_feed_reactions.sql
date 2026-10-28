CREATE TABLE feed_reactions (
  feed_object_id uuid NOT NULL REFERENCES feed_objects (id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  emoji text NOT NULL CHECK (octet_length(emoji) BETWEEN 1 AND 32),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (feed_object_id, user_id, emoji)
);

CREATE INDEX feed_reactions_item_emoji_idx ON feed_reactions (feed_object_id, emoji, created_at);
CREATE INDEX feed_reactions_user_idx ON feed_reactions (user_id);

INSERT INTO feed_reactions (feed_object_id, user_id, emoji, created_at)
SELECT feed_object_id, user_id, U&'\2764\FE0F', created_at FROM feed_likes;
