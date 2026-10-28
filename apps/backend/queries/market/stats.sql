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


-- name: YearRange :one
WITH raw AS (
  SELECT (p.ts AT TIME ZONE 'UTC')::date AS day, p.price_micros
  FROM price_points AS p
  LEFT JOIN LATERAL (
    SELECT prev.price_micros
    FROM price_points AS prev
    WHERE prev.mint = p.mint AND prev.ts < p.ts
    ORDER BY prev.ts DESC
    LIMIT 1
  ) AS earlier ON true
  WHERE p.mint = sqlc.arg(mint)::text
    AND p.ts >= ((sqlc.arg(after)::timestamptz AT TIME ZONE 'UTC')::date + 1)::timestamp AT TIME ZONE 'UTC'
    AND p.price_micros > 0
    AND (earlier.price_micros IS NULL OR abs(p.price_micros - earlier.price_micros) <= earlier.price_micros / 5)
),
days AS (
  SELECT r.day, max(r.price_micros) AS high_micros, min(r.price_micros) AS low_micros
  FROM raw AS r
  GROUP BY r.day
  UNION ALL
  SELECT d.day, d.high_micros, d.low_micros
  FROM price_day_ranges AS d
  WHERE d.mint = sqlc.arg(mint)::text
    AND d.day > (sqlc.arg(after)::timestamptz AT TIME ZONE 'UTC')::date
    AND NOT (d.source = 'history' AND d.day IN (SELECT r.day FROM raw AS r))
)
SELECT
  coalesce(min(day) - (sqlc.arg(after)::timestamptz AT TIME ZONE 'UTC')::date, 0)::integer AS oldest_gap_days,
  coalesce(min(low_micros), 0)::bigint AS low_micros,
  coalesce(max(high_micros), 0)::bigint AS high_micros
FROM days;
