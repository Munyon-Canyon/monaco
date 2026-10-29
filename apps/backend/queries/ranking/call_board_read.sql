-- name: CallBoardTop :many
SELECT board, period, period_start, subject_id, subject_kind, subject_name, subject_handle, subject_picture_url,
  subject_created_at, rank, right_count, resolved, voted, eligible, rate_bps, turnout_bps, baseline_rank, movement,
  movement_kind, last_call, run_id, computed_at
FROM call_board_entries
WHERE board = sqlc.arg(board) AND period = sqlc.arg(period) AND rank BETWEEN 1 AND 3
ORDER BY rank;

-- name: CallBoardRow :one
SELECT board, period, period_start, subject_id, subject_kind, subject_name, subject_handle, subject_picture_url,
  subject_created_at, rank, right_count, resolved, voted, eligible, rate_bps, turnout_bps, baseline_rank, movement,
  movement_kind, last_call, run_id, computed_at
FROM call_board_entries
WHERE board = sqlc.arg(board) AND period = sqlc.arg(period) AND subject_id = sqlc.arg(subject);

-- name: CallBoardAt :one
SELECT board, period, period_start, subject_id, subject_kind, subject_name, subject_handle, subject_picture_url,
  subject_created_at, rank, right_count, resolved, voted, eligible, rate_bps, turnout_bps, baseline_rank, movement,
  movement_kind, last_call, run_id, computed_at
FROM call_board_entries
WHERE board = sqlc.arg(board) AND period = sqlc.arg(period) AND rank = sqlc.arg(rank)::int;

-- name: CallBoardWindow :many
SELECT board, period, period_start, subject_id, subject_kind, subject_name, subject_handle, subject_picture_url,
  subject_created_at, rank, right_count, resolved, voted, eligible, rate_bps, turnout_bps, baseline_rank, movement,
  movement_kind, last_call, run_id, computed_at
FROM call_board_entries
WHERE board = sqlc.arg(board) AND period = sqlc.arg(period)
  AND rank BETWEEN sqlc.arg(from_rank)::int AND sqlc.arg(to_rank)::int
ORDER BY rank;

-- name: CallBoardRanked :many
SELECT board, period, period_start, subject_id, subject_kind, subject_name, subject_handle, subject_picture_url,
  subject_created_at, rank, right_count, resolved, voted, eligible, rate_bps, turnout_bps, baseline_rank, movement,
  movement_kind, last_call, run_id, computed_at
FROM call_board_entries
WHERE board = sqlc.arg(board) AND period = sqlc.arg(period) AND rank IS NOT NULL
  AND subject_id = ANY(sqlc.arg(subjects)::uuid[])
ORDER BY rank;

-- name: CallBoardOf :many
SELECT board, period, period_start, subject_id, subject_kind, subject_name, subject_handle, subject_picture_url,
  subject_created_at, rank, right_count, resolved, voted, eligible, rate_bps, turnout_bps, baseline_rank, movement,
  movement_kind, last_call, run_id, computed_at
FROM call_board_entries
WHERE board = sqlc.arg(board) AND period = sqlc.arg(period) AND subject_id = ANY(sqlc.arg(subjects)::uuid[])
ORDER BY rank NULLS LAST, resolved DESC, subject_name, subject_id;

-- name: CallBoardUnranked :many
SELECT board, period, period_start, subject_id, subject_kind, subject_name, subject_handle, subject_picture_url,
  subject_created_at, rank, right_count, resolved, voted, eligible, rate_bps, turnout_bps, baseline_rank, movement,
  movement_kind, last_call, run_id, computed_at
FROM call_board_entries
WHERE board = sqlc.arg(board) AND period = sqlc.arg(period) AND rank IS NULL
  AND resolved >= sqlc.arg(min_resolved)::int
  AND subject_id = ANY(sqlc.arg(subjects)::uuid[])
ORDER BY resolved DESC, subject_name, subject_id
LIMIT sqlc.arg(row_limit)::int;

-- name: CallBoardCount :one
SELECT count(*)::int AS ranked
FROM call_board_entries
WHERE board = sqlc.arg(board) AND period = sqlc.arg(period) AND rank IS NOT NULL;
