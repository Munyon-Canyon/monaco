CREATE TABLE swap_operator_actions (
  id uuid PRIMARY KEY,
  swap_id uuid NOT NULL REFERENCES swaps (id),
  action text NOT NULL CHECK (action IN ('unpark', 'fail_parked')),
  actor text NOT NULL CHECK (actor <> ''),
  reason text NOT NULL CHECK (char_length(reason) BETWEEN 1 AND 500),
  detail jsonb NOT NULL,
  created_at timestamptz NOT NULL
);

CREATE INDEX swap_operator_actions_swap ON swap_operator_actions (swap_id, created_at);
