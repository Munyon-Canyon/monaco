ALTER TABLE cabal_messages ALTER COLUMN author_id DROP NOT NULL;

ALTER TABLE cabal_messages
  ADD COLUMN kind text NOT NULL DEFAULT 'user' CHECK (kind IN ('user', 'proposal', 'system')),
  ADD COLUMN moment jsonb,
  ADD COLUMN dedupe_key text;

ALTER TABLE cabal_messages ADD CONSTRAINT cabal_messages_shape_check CHECK (
  (kind = 'user' AND author_id IS NOT NULL AND moment IS NULL)
  OR (kind = 'proposal' AND author_id IS NOT NULL AND proposal_id IS NOT NULL AND moment IS NULL)
  OR (kind = 'system' AND author_id IS NULL AND body IS NULL AND moment IS NOT NULL AND parent_id IS NULL)
);

CREATE UNIQUE INDEX cabal_messages_dedupe_idx ON cabal_messages (cabal_id, dedupe_key) WHERE dedupe_key IS NOT NULL;
