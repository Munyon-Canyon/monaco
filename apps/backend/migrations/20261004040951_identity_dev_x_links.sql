CREATE TABLE dev_x_links (
  user_id uuid PRIMARY KEY REFERENCES users (id),
  x_user_id text NOT NULL UNIQUE,
  x_username text NOT NULL,
  created_at timestamptz NOT NULL
);
