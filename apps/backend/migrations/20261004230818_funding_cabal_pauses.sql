CREATE TABLE cabal_pauses (
  id uuid PRIMARY KEY,
  cabal_id uuid REFERENCES cabals (id),
  reason text NOT NULL CHECK (reason IN ('external_deposit', 'ops')),
  note text NOT NULL DEFAULT '',
  created_by uuid,
  created_at timestamptz NOT NULL,
  resolved_at timestamptz,
  resolved_by uuid,
  CHECK (resolved_by IS NULL OR resolved_at IS NOT NULL)
);

CREATE INDEX cabal_pauses_open_idx ON cabal_pauses (cabal_id) WHERE resolved_at IS NULL;
