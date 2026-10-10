CREATE TABLE admin_approvals (
  id uuid PRIMARY KEY,
  action text NOT NULL CHECK (action IN ('cabal_ban')),
  target_id uuid NOT NULL,
  requested_by uuid NOT NULL,
  reason text NOT NULL,
  status text NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'expired')),
  decided_by uuid,
  decided_reason text,
  created_at timestamptz NOT NULL,
  decided_at timestamptz,
  expires_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX admin_approvals_pending_idx ON admin_approvals (action, target_id) WHERE status = 'pending';
CREATE INDEX admin_approvals_status_idx ON admin_approvals (status, id DESC);
