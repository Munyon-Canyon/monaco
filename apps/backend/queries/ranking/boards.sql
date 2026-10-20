-- name: BoardPage :many
SELECT board, range, rank, subject_id, subject_name, subject_handle, subject_picture_url, subject_created_at,
  value_micros, pnl_micros, return_bps, prices_as_of, computed_at, flags, join_mode, member_count
FROM leaderboard_entries
WHERE board = sqlc.arg(board) AND range = sqlc.arg(range) AND rank > sqlc.arg(after_rank)
ORDER BY rank
LIMIT sqlc.arg(row_limit);

-- name: BoardRowFor :one
SELECT board, range, rank, subject_id, subject_name, subject_handle, subject_picture_url, subject_created_at,
  value_micros, pnl_micros, return_bps, prices_as_of, computed_at, flags, join_mode, member_count
FROM leaderboard_entries
WHERE board = sqlc.arg(board) AND range = sqlc.arg(range) AND subject_id = sqlc.arg(subject_id);

-- name: BoardRowsForSubjects :many
SELECT board, range, rank, subject_id, subject_name, subject_handle, subject_picture_url, subject_created_at,
  value_micros, pnl_micros, return_bps, prices_as_of, computed_at, flags, join_mode, member_count
FROM leaderboard_entries
WHERE board = sqlc.arg(board) AND range = sqlc.arg(range) AND subject_id = ANY(sqlc.arg(subject_ids)::uuid[])
ORDER BY rank;

-- name: BoardViewPage :many
SELECT board, range, position::int AS rank, subject_id, subject_name, subject_handle, subject_picture_url,
  subject_created_at, value_micros, pnl_micros, return_bps, prices_as_of, computed_at, flags, join_mode, member_count
FROM (
  SELECT e.board, e.range, e.rank, e.subject_id, e.subject_name, e.subject_handle, e.subject_picture_url,
    e.subject_created_at, e.value_micros, e.pnl_micros, e.return_bps, e.prices_as_of, e.computed_at, e.flags,
    e.join_mode, e.member_count, row_number() OVER (
    ORDER BY (CASE WHEN sqlc.arg(sort)::text = 'pnl' THEN e.pnl_micros END) DESC NULLS LAST,
      (CASE WHEN sqlc.arg(sort)::text = 'name' THEN lower(e.subject_name) END) ASC NULLS LAST,
      e.rank
  ) AS position
  FROM leaderboard_entries e
  WHERE e.board = sqlc.arg(board) AND e.range = sqlc.arg(range)
    AND (cardinality(sqlc.arg(join_modes)::text[]) = 0 OR e.join_mode = ANY(sqlc.arg(join_modes)::text[]))
    AND e.member_count >= sqlc.arg(min_members)::int
) viewed
WHERE position > sqlc.arg(after_position)::int
ORDER BY position
LIMIT sqlc.arg(row_limit);

-- name: BoardViewRowFor :one
SELECT board, range, position::int AS rank, subject_id, subject_name, subject_handle, subject_picture_url,
  subject_created_at, value_micros, pnl_micros, return_bps, prices_as_of, computed_at, flags, join_mode, member_count
FROM (
  SELECT e.board, e.range, e.rank, e.subject_id, e.subject_name, e.subject_handle, e.subject_picture_url,
    e.subject_created_at, e.value_micros, e.pnl_micros, e.return_bps, e.prices_as_of, e.computed_at, e.flags,
    e.join_mode, e.member_count, row_number() OVER (
    ORDER BY (CASE WHEN sqlc.arg(sort)::text = 'pnl' THEN e.pnl_micros END) DESC NULLS LAST,
      (CASE WHEN sqlc.arg(sort)::text = 'name' THEN lower(e.subject_name) END) ASC NULLS LAST,
      e.rank
  ) AS position
  FROM leaderboard_entries e
  WHERE e.board = sqlc.arg(board) AND e.range = sqlc.arg(range)
) viewed
WHERE subject_id = sqlc.arg(subject_id);
