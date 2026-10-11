UPDATE asset_listings SET tradable_override = NULL, enabled = verified_at IS NOT NULL AND issuer_tradable
WHERE network = 'robinhood' AND tradable_override = false;
