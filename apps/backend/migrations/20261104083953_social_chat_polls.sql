CREATE TABLE chat_poll_options (
  message_id uuid NOT NULL REFERENCES chat_message_attachments (message_id) ON DELETE CASCADE,
  position smallint NOT NULL CHECK (position BETWEEN 0 AND 3),
  label text NOT NULL CHECK (char_length(label) BETWEEN 1 AND 40),
  PRIMARY KEY (message_id, position)
);

CREATE TABLE chat_poll_votes (
  message_id uuid NOT NULL,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  position smallint NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (message_id, user_id),
  FOREIGN KEY (message_id, position) REFERENCES chat_poll_options (message_id, position) ON DELETE CASCADE
);

CREATE INDEX chat_poll_votes_option_idx ON chat_poll_votes (message_id, position);
CREATE INDEX chat_poll_votes_user_idx ON chat_poll_votes (user_id);
