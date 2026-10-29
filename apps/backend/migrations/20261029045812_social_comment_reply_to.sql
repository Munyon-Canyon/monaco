ALTER TABLE feed_comments ADD COLUMN reply_to_comment_id uuid;

ALTER TABLE feed_comments ADD CONSTRAINT feed_comments_reply_to_fk
  FOREIGN KEY (feed_object_id, reply_to_comment_id) REFERENCES feed_comments (feed_object_id, id);

UPDATE feed_comments SET reply_to_comment_id = parent_comment_id WHERE parent_comment_id IS NOT NULL;
