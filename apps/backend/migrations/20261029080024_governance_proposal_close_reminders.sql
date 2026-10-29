CREATE TABLE proposal_close_reminders (
  proposal_id uuid NOT NULL REFERENCES proposals (id),
  user_id uuid NOT NULL REFERENCES users (id),
  due_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL,
  sent_at timestamptz,
  PRIMARY KEY (proposal_id, user_id)
);

CREATE INDEX proposal_close_reminders_due_idx ON proposal_close_reminders (due_at) WHERE sent_at IS NULL;
