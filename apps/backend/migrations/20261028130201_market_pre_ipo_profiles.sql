CREATE TABLE pre_ipo_profiles (
  company_key text PRIMARY KEY,
  estimate_micros bigint NOT NULL CHECK (estimate_micros > 0),
  estimate_on date NOT NULL,
  valuation_micros numeric(30, 0) NOT NULL CHECK (valuation_micros > 0),
  min_order_micros bigint NOT NULL CHECK (min_order_micros > 0),
  sell_window text NOT NULL CHECK (sell_window IN ('any_time', 'tender_quarterly')),
  next_tender_on date,
  image_url text,
  updated_at timestamptz NOT NULL
);

CREATE TABLE pre_ipo_rounds (
  company_key text NOT NULL REFERENCES pre_ipo_profiles (company_key),
  round_on date NOT NULL,
  price_micros bigint NOT NULL CHECK (price_micros > 0),
  PRIMARY KEY (company_key, round_on)
);

-- Seed profiles come from the issuers' public catalogs, captured 2026-10-28: estimate_micros is the issuer's
-- markPrice per token and valuation_micros is its markValuation in USD. Neither issuer publishes funding rounds
-- or tender dates, so pre_ipo_rounds starts empty and next_tender_on is null. PreStocks wins where both list a
-- company.
INSERT INTO pre_ipo_profiles
  (company_key, estimate_micros, estimate_on, valuation_micros, min_order_micros, sell_window, updated_at)
VALUES
  -- https://prestocks.com/api/prestocks
  ('anduril', 143342292, DATE '2026-10-28', 126814790637000000, 10000000, 'any_time',
    TIMESTAMPTZ '2026-10-28 12:00:00+00'),
  -- https://prestocks.com/api/prestocks
  ('anthropic', 976291629, DATE '2026-10-28', 1599501233187000000, 10000000, 'any_time',
    TIMESTAMPTZ '2026-10-28 12:00:00+00'),
  -- https://prestocks.com/api/prestocks
  ('figure ai', 187092238, DATE '2026-10-28', 40791156350000000, 10000000, 'any_time',
    TIMESTAMPTZ '2026-10-28 12:00:00+00'),
  -- https://prestocks.com/api/prestocks
  ('kalshi', 961298926, DATE '2026-10-28', 34964282572000000, 10000000, 'any_time',
    TIMESTAMPTZ '2026-10-28 12:00:00+00'),
  -- https://prestocks.com/api/prestocks
  ('neuralink', 331892957, DATE '2026-10-28', 63223965217000000, 10000000, 'any_time',
    TIMESTAMPTZ '2026-10-28 12:00:00+00'),
  -- https://prestocks.com/api/prestocks
  ('openai', 1089325038, DATE '2026-10-28', 1349597794332000000, 10000000, 'any_time',
    TIMESTAMPTZ '2026-10-28 12:00:00+00'),
  -- https://prestocks.com/api/prestocks
  ('oura', 73540589, DATE '2026-10-28', 15100737010000000, 10000000, 'any_time',
    TIMESTAMPTZ '2026-10-28 12:00:00+00'),
  -- https://prestocks.com/api/prestocks
  ('polymarket', 139405988, DATE '2026-10-28', 14513867583000000, 10000000, 'any_time',
    TIMESTAMPTZ '2026-10-28 12:00:00+00'),
  -- https://rest-api.tessera.pe/v1/public/token-details
  ('spacex', 423000000, DATE '2026-10-28', 800000000000000000, 10000000, 'any_time',
    TIMESTAMPTZ '2026-10-28 12:00:00+00');
