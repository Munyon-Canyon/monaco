-- name: MarketStats :one
SELECT
  coalesce((
    SELECT (shares_outstanding * sqlc.arg(price_micros)::bigint)::text
    FROM company_facts
    WHERE company_key = sqlc.arg(company_key)::text AND sqlc.arg(price_micros)::bigint > 0
  ), '')::text AS market_cap_micros,
  coalesce((
    SELECT first_samples.price_micros
    FROM (
      SELECT ts, price_micros, lead(price_micros) OVER (ORDER BY ts) AS next_micros
      FROM price_points
      WHERE mint = sqlc.arg(mint)::text
        AND ts >= sqlc.arg(since)::timestamptz
        AND ts <= sqlc.arg(until)::timestamptz
      ORDER BY ts
      LIMIT 3
    ) AS first_samples
    WHERE first_samples.next_micros IS NULL
      OR abs(first_samples.price_micros - first_samples.next_micros) * 5 <= first_samples.price_micros
    ORDER BY first_samples.ts
    LIMIT 1
  ), 0)::bigint AS open_micros;
