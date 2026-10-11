ALTER TABLE asset_listings
  ADD COLUMN issuer_tradable bool NOT NULL DEFAULT true,
  ADD COLUMN tradable_override bool;

UPDATE asset_listings SET issuer_tradable = enabled WHERE verified_at IS NOT NULL;

UPDATE asset_listings SET tradable_override = false, enabled = false
WHERE issuer = 'xstocks' AND network <> 'solana';

ALTER TABLE proposals ADD COLUMN price_comparison jsonb;
