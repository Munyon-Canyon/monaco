-- name: PreIpoProfiles :many
SELECT company_key, estimate_micros, estimate_on, valuation_micros::text AS valuation_micros, min_order_micros,
  sell_window, next_tender_on, image_url
FROM pre_ipo_profiles
WHERE company_key = ANY (sqlc.arg(company_keys)::text[]);

-- name: PreIpoRounds :many
SELECT company_key, round_on, price_micros
FROM pre_ipo_rounds
WHERE company_key = ANY (sqlc.arg(company_keys)::text[])
ORDER BY company_key, round_on;
