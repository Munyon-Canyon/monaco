CREATE TABLE assets (
  id uuid PRIMARY KEY,
  symbol text UNIQUE NOT NULL,
  mint text UNIQUE NOT NULL,
  decimals smallint NOT NULL,
  issuer text NOT NULL CHECK (issuer IN ('xstocks', 'tessera', 'prestocks')),
  kind text NOT NULL CHECK (kind IN ('equity', 'pre_ipo')),
  display_name text NOT NULL,
  logo_url text,
  ui_multiplier_num bigint NOT NULL DEFAULT 1,
  ui_multiplier_den bigint NOT NULL DEFAULT 1,
  issuer_tradable bool NOT NULL,
  tradable_override bool NULL,
  popular_rank smallint NULL,
  company_key text NOT NULL,
  first_seen_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);
