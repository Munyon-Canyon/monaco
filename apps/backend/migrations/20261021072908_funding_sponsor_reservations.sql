CREATE TABLE relayer_sponsor_reservations (
  ref uuid NOT NULL,
  scope text NOT NULL CHECK (scope IN ('user', 'destination', 'global')),
  scope_key text NOT NULL,
  lamports numeric(20, 0) NOT NULL CHECK (lamports > 0),
  rent boolean NOT NULL,
  created_at timestamptz NOT NULL,
  released_at timestamptz,
  PRIMARY KEY (ref, scope)
);

CREATE INDEX relayer_sponsor_reservations_scope_created ON relayer_sponsor_reservations (scope, scope_key, created_at DESC);
