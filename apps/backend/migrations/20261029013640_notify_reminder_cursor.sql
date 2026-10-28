CREATE TABLE notify_reminder_cursor (
  name text PRIMARY KEY,
  due_at timestamptz NOT NULL,
  proposal_id uuid NOT NULL
);
