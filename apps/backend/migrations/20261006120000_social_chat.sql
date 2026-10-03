CREATE TABLE cabal_messages (
  id uuid PRIMARY KEY,
  cabal_id uuid NOT NULL REFERENCES cabals (id),
  author_id uuid NOT NULL REFERENCES users (id),
  body text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 2000),
  created_at timestamptz NOT NULL,
  parent_id uuid,
  also_in_channel boolean NOT NULL DEFAULT false,
  reply_count int NOT NULL DEFAULT 0,
  last_reply_at timestamptz,
  proposal_id uuid,
  deleted_at timestamptz,
  UNIQUE (cabal_id, id),
  FOREIGN KEY (cabal_id, parent_id) REFERENCES cabal_messages (cabal_id, id),
  CHECK (also_in_channel = false OR parent_id IS NOT NULL)
);

CREATE INDEX cabal_messages_channel_idx ON cabal_messages (cabal_id, created_at DESC, id DESC);

CREATE INDEX cabal_messages_thread_idx ON cabal_messages (parent_id, created_at, id);
