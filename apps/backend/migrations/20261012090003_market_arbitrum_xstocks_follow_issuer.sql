UPDATE asset_listings SET tradable_override = NULL, enabled = verified_at IS NOT NULL AND issuer_tradable
WHERE issuer = 'xstocks' AND network = 'arbitrum' AND (tradable_override = false OR (tradable_override IS NULL
  AND enabled IS DISTINCT FROM (verified_at IS NOT NULL AND issuer_tradable)));
