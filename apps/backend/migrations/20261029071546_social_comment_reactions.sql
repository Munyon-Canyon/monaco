CREATE TABLE comment_reactions (
  comment_id uuid NOT NULL,
  feed_object_id uuid NOT NULL,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  emoji text NOT NULL CHECK (octet_length(emoji) BETWEEN 1 AND 32),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (comment_id, user_id, emoji),
  FOREIGN KEY (feed_object_id, comment_id) REFERENCES feed_comments (feed_object_id, id) ON DELETE CASCADE
);

CREATE INDEX comment_reactions_comment_emoji_idx ON comment_reactions (comment_id, emoji, created_at);
