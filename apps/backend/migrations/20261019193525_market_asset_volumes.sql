CREATE TABLE asset_volumes (
  mint text PRIMARY KEY,
  volume_24h_micros bigint NOT NULL CHECK (volume_24h_micros > 0),
  as_of timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);
