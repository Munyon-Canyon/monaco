DO $$
DECLARE
  mismatched text;
BEGIN
  SELECT string_agg(a.symbol || ' ' || a.mint || ' <> ' || l.token_address, ', ' ORDER BY a.symbol)
  INTO mismatched
  FROM assets AS a
  JOIN asset_listings AS l ON l.issuer = a.issuer AND l.symbol = a.symbol AND l.network = 'solana'
  WHERE l.token_address <> a.mint;
  IF mismatched IS NOT NULL THEN
    RAISE EXCEPTION 'asset_listings: Solana mints differ from assets: %', mismatched;
  END IF;
END
$$;

UPDATE asset_listings AS l SET asset_id = a.id
FROM assets AS a
WHERE l.asset_id IS NULL AND a.issuer = l.issuer AND a.symbol = l.symbol;

UPDATE cabal_positions AS p SET listing_id = l.id
FROM asset_listings AS l
WHERE p.listing_id IS NULL AND l.network = 'solana' AND l.token_address = p.asset;
