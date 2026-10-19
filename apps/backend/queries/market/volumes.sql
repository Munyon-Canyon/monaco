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
