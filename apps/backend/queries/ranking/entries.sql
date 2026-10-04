-- name: DeleteAllLeaderboardEntries :exec
DELETE FROM leaderboard_entries WHERE range = 'ALL';

-- name: InsertLeaderboardEntries :exec
INSERT INTO leaderboard_entries (
  board, range, rank, subject_id, subject_name, subject_handle, subject_picture_url, subject_created_at,
  value_micros, pnl_micros, return_bps, prices_as_of, computed_at, flags
)
SELECT board, range, rank, subject_id, subject_name, subject_handle, subject_picture_url, subject_created_at,
  value_micros, pnl_micros, return_bps, prices_as_of, computed_at, flags
FROM jsonb_to_recordset(sqlc.arg(rows)::jsonb) AS rows(
  board text, range text, rank int, subject_id uuid, subject_name text, subject_handle text,
  subject_picture_url text, subject_created_at timestamptz, value_micros bigint, pnl_micros bigint,
  return_bps bigint, prices_as_of timestamptz, computed_at timestamptz, flags text[]
);
