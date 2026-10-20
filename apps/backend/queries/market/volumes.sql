-- name: UpsertAssetVolume :exec
INSERT INTO asset_volumes (mint, volume_24h_micros, as_of, updated_at)
VALUES (sqlc.arg(mint)::text, sqlc.arg(volume_24h_micros)::bigint, sqlc.arg(as_of)::timestamptz,
  sqlc.arg(updated_at)::timestamptz)
ON CONFLICT (mint) DO UPDATE SET
  volume_24h_micros = excluded.volume_24h_micros,
  as_of = excluded.as_of,
  updated_at = excluded.updated_at
WHERE asset_volumes.as_of < excluded.as_of;

-- name: VolumesByMints :many
SELECT mint, volume_24h_micros, as_of
FROM asset_volumes
WHERE mint = ANY (sqlc.arg(mints)::text[]) AND as_of >= sqlc.arg(since)::timestamptz;

-- name: RecordCoinGeckoCall :one
INSERT INTO coingecko_usage (month, calls, updated_at)
VALUES (date_trunc('month', sqlc.arg(at)::timestamptz AT TIME ZONE 'UTC')::date, 1, sqlc.arg(at)::timestamptz)
ON CONFLICT (month) DO UPDATE SET
  calls = coingecko_usage.calls + 1,
  updated_at = excluded.updated_at
RETURNING calls;

-- name: CoinGeckoCalls :one
SELECT coalesce(max(calls), 0)::integer AS calls
FROM coingecko_usage
WHERE month = date_trunc('month', sqlc.arg(at)::timestamptz AT TIME ZONE 'UTC')::date;
