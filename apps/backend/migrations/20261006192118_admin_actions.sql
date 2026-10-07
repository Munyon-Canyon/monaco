CREATE TABLE admin_actions (
  id uuid PRIMARY KEY,
  admin_id uuid NOT NULL,
  action text NOT NULL,
  target_type text NOT NULL,
  target_id text NOT NULL,
  reason text NOT NULL,
  before jsonb NOT NULL,
  after jsonb NOT NULL,
  approved_by uuid,
  created_at timestamptz NOT NULL
);
CREATE INDEX admin_actions_target_idx ON admin_actions (target_type, target_id, id DESC);
CREATE INDEX admin_actions_admin_idx ON admin_actions (admin_id, id DESC);
