CREATE TABLE system_pings (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL,
  note text NOT NULL CHECK (char_length(note) <= 140),
  echoed_at timestamptz
);
