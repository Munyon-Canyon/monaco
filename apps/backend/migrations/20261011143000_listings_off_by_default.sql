UPDATE asset_listings SET tradable_override = false, enabled = false
WHERE tradable_override IS NULL AND ((issuer = 'xstocks' AND network <> 'solana') OR network = 'robinhood');
