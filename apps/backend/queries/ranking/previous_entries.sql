-- name: PreviousEntriesForCabals :many
SELECT board, range, rank, subject_id, subject_name, subject_handle, subject_picture_url, subject_created_at,
  value_micros, pnl_micros, return_bps, prices_as_of, computed_at, flags
FROM leaderboard_entries
WHERE (board = 'cabals' AND subject_id = ANY(sqlc.arg(cabal_ids)::uuid[]))
  OR board = ANY(sqlc.arg(member_boards)::text[])
ORDER BY board, range, rank;
