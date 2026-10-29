ALTER TABLE feed_comments ALTER COLUMN body DROP NOT NULL;

ALTER TABLE feed_comments DROP CONSTRAINT feed_comments_body_check;

ALTER TABLE feed_comments ADD CONSTRAINT feed_comments_body_check
  CHECK (body IS NULL OR char_length(body) BETWEEN 1 AND 1000);

CREATE TABLE comment_attachments (
  comment_id uuid PRIMARY KEY,
  feed_object_id uuid NOT NULL,
  kind text NOT NULL CHECK (kind IN ('gif', 'poll', 'stock', 'thesis_quote')),
  payload jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (feed_object_id, comment_id) REFERENCES feed_comments (feed_object_id, id) ON DELETE CASCADE
);
