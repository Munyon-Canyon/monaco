ALTER TABLE deposit_watch_wallets
  ADD COLUMN opening_micros numeric(20,0),
  ADD COLUMN reconcile_due_at timestamptz,
  ADD COLUMN residual_streak integer NOT NULL DEFAULT 0;

UPDATE deposit_watch_wallets SET reconcile_due_at = now() WHERE reconcile_due_at IS NULL;
