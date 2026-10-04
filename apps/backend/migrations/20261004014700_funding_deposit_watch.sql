CREATE TABLE deposit_watch_wallets (
  wallet_address text PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id),
  first_seen_slot bigint NOT NULL,
  first_seen_at timestamptz NOT NULL,
  discovery_due_at timestamptz
);

CREATE TABLE deposit_watch_accounts (
  token_account text PRIMARY KEY,
  wallet_address text NOT NULL REFERENCES deposit_watch_wallets(wallet_address),
  canonical boolean NOT NULL,
  state text NOT NULL CHECK (state IN ('missing', 'open', 'closed', 'foreign')),
  last_amount numeric(20,0) NOT NULL DEFAULT 0,
  observed_slot bigint NOT NULL DEFAULT 0,
  dirty_gen bigint NOT NULL DEFAULT 0,
  clean_gen bigint NOT NULL DEFAULT 0,
  dirty_slot bigint NOT NULL DEFAULT 0,
  high_signature text,
  high_slot bigint NOT NULL DEFAULT 0,
  page_before text,
  page_top_signature text,
  page_top_slot bigint,
  recovery_before text,
  recovery_due_at timestamptz NOT NULL,
  scanned_at timestamptz
);

CREATE INDEX deposit_watch_accounts_dirty_idx
ON deposit_watch_accounts (dirty_slot, token_account)
WHERE dirty_gen > clean_gen;
