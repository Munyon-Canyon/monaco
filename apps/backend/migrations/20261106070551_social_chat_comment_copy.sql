ALTER TABLE cabal_messages
  ADD COLUMN comment_id uuid REFERENCES feed_comments (id);

CREATE UNIQUE INDEX cabal_messages_comment_idx ON cabal_messages (comment_id) WHERE comment_id IS NOT NULL;
