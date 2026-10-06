CREATE TABLE contact_matches (
  user_id uuid NOT NULL,
  matched_user_id uuid NOT NULL,
  source text NOT NULL CHECK (source IN ('phone', 'x')),
  created_at timestamptz NOT NULL,
  dismissed_at timestamptz,
  PRIMARY KEY (user_id, matched_user_id, source)
);

CREATE INDEX contact_matches_live_idx
  ON contact_matches (user_id, created_at DESC)
  WHERE dismissed_at IS NULL;
