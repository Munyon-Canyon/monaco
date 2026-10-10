ALTER TABLE feed_comments
  ADD COLUMN deleted_by_kind text NOT NULL DEFAULT 'author' CHECK (deleted_by_kind IN ('author', 'admin'));
