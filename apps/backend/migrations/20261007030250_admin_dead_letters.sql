CREATE TABLE dead_letters (
  id uuid PRIMARY KEY,
  stream_seq bigint NOT NULL UNIQUE,
  consumer text NOT NULL,
  handler text NOT NULL DEFAULT '',
  subject text NOT NULL DEFAULT '',
  event_id uuid,
  code text NOT NULL,
  error text NOT NULL DEFAULT '',
  letter jsonb NOT NULL,
  occurrences int NOT NULL DEFAULT 1,
  status text NOT NULL CHECK (status IN ('open', 'redriven', 'resolved', 'discarded')),
  first_seen_at timestamptz NOT NULL,
  last_seen_at timestamptz NOT NULL,
  redriven_at timestamptz,
  resolved_at timestamptz,
  resolved_by uuid,
  resolve_reason text
);
CREATE UNIQUE INDEX dead_letters_live_idx ON dead_letters (consumer, event_id)
  WHERE event_id IS NOT NULL AND status IN ('open', 'redriven');
CREATE INDEX dead_letters_status_idx ON dead_letters (status, id DESC);
