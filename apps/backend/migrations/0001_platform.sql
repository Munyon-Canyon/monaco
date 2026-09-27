CREATE TABLE events (
  id uuid PRIMARY KEY,
  aggregate_type text NOT NULL,
  aggregate_id uuid NOT NULL,
  type text NOT NULL,
  payload jsonb NOT NULL CHECK (payload ? 'v'),
  actor_type text NOT NULL CHECK (actor_type IN ('user', 'admin', 'system', 'agent')),
  actor_id text NOT NULL,
  trace_parent text,
  created_at timestamptz NOT NULL DEFAULT now(),
  published_at timestamptz
);

CREATE INDEX events_unpublished_idx ON events (id) WHERE published_at IS NULL;

CREATE TABLE event_deliveries (
  handler text NOT NULL,
  event_id uuid NOT NULL REFERENCES events (id),
  code text NOT NULL,
  handled_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (handler, event_id)
);

CREATE INDEX event_deliveries_handled_at_idx ON event_deliveries (handled_at);
