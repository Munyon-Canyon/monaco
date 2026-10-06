CREATE TABLE chat_seen (
  cabal_id uuid NOT NULL,
  user_id uuid NOT NULL,
  last_seen_at timestamptz NOT NULL,
  seen_published_at timestamptz,
  PRIMARY KEY (cabal_id, user_id)
);

CREATE INDEX chat_seen_cabal_last_seen_idx ON chat_seen (cabal_id, last_seen_at);
