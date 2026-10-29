ALTER TABLE feed_comments
  ADD COLUMN author_kind text NOT NULL DEFAULT 'user' CHECK (author_kind IN ('user', 'system')),
  ADD COLUMN system_outcome text,
  ALTER COLUMN author_id DROP NOT NULL,
  ADD CONSTRAINT feed_comments_author_kind_author_id CHECK ((author_kind = 'user') = (author_id IS NOT NULL));

CREATE UNIQUE INDEX feed_comments_system_outcome_idx
  ON feed_comments (feed_object_id, system_outcome) WHERE author_kind = 'system';
