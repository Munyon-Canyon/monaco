-- name: BoardPage :many
SELECT board, range, rank, subject_id, subject_name, subject_handle, subject_picture_url, subject_created_at,
  value_micros, pnl_micros, return_bps, prices_as_of, computed_at, flags
FROM leaderboard_entries
WHERE board = sqlc.arg(board) AND range = sqlc.arg(range) AND rank > sqlc.arg(after_rank)
ORDER BY rank
LIMIT sqlc.arg(row_limit);

-- name: BoardRowFor :one
SELECT board, range, rank, subject_id, subject_name, subject_handle, subject_picture_url, subject_created_at,
  value_micros, pnl_micros, return_bps, prices_as_of, computed_at, flags
FROM leaderboard_entries
WHERE board = sqlc.arg(board) AND range = sqlc.arg(range) AND subject_id = sqlc.arg(subject_id);

-- name: BoardRowsForSubjects :many
SELECT board, range, rank, subject_id, subject_name, subject_handle, subject_picture_url, subject_created_at,
  value_micros, pnl_micros, return_bps, prices_as_of, computed_at, flags
FROM leaderboard_entries
WHERE board = sqlc.arg(board) AND range = sqlc.arg(range) AND subject_id = ANY(sqlc.arg(subject_ids)::uuid[])
ORDER BY rank;
