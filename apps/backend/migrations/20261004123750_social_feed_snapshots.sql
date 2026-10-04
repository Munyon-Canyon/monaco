CREATE TABLE feed_cabals (
  cabal_id uuid PRIMARY KEY,
  name text NOT NULL,
  updated_at timestamptz NOT NULL
);

CREATE TABLE feed_memberships (
  cabal_id uuid NOT NULL REFERENCES feed_cabals (cabal_id),
  user_id uuid NOT NULL,
  joined_at timestamptz NOT NULL,
  PRIMARY KEY (cabal_id, user_id)
);
