CREATE TABLE comment_poll_options (
  comment_id uuid NOT NULL REFERENCES comment_attachments (comment_id) ON DELETE CASCADE,
  position smallint NOT NULL CHECK (position BETWEEN 0 AND 3),
  label text NOT NULL CHECK (char_length(label) BETWEEN 1 AND 40),
  PRIMARY KEY (comment_id, position)
);

CREATE TABLE comment_poll_votes (
  comment_id uuid NOT NULL,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  position smallint NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (comment_id, user_id),
  FOREIGN KEY (comment_id, position) REFERENCES comment_poll_options (comment_id, position) ON DELETE CASCADE
);

CREATE INDEX comment_poll_votes_option_idx ON comment_poll_votes (comment_id, position);
CREATE INDEX comment_poll_votes_user_idx ON comment_poll_votes (user_id);
