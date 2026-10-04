CREATE TABLE admins (
  user_id uuid PRIMARY KEY,
  role text NOT NULL CHECK (role IN ('viewer', 'moderator', 'operator')),
  granted_by uuid,
  granted_at timestamptz NOT NULL,
  revoked_at timestamptz
);
