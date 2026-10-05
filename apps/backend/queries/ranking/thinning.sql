-- name: ThinCabalValueSnapshots :execrows
DELETE FROM cabal_value_snapshots
WHERE (cabal_id, at) IN (
  SELECT ranked.cabal_id, ranked.at
  FROM (
    SELECT cabal_id, at,
      row_number() OVER (PARTITION BY cabal_id, date_trunc('hour', at) ORDER BY at DESC) AS position
    FROM cabal_value_snapshots
    WHERE at < sqlc.arg(before)::timestamptz
  ) AS ranked
  WHERE ranked.position > 1
);
