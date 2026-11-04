CREATE TABLE chat_message_reactions (
  message_id uuid NOT NULL,
  cabal_id uuid NOT NULL,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  emoji text NOT NULL CHECK (octet_length(emoji) BETWEEN 1 AND 32),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (message_id, user_id, emoji),
  FOREIGN KEY (cabal_id, message_id) REFERENCES cabal_messages (cabal_id, id) ON DELETE CASCADE
);

CREATE INDEX chat_message_reactions_emoji_idx ON chat_message_reactions (message_id, emoji, created_at);
