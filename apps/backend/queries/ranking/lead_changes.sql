-- name: CallBoardRanksOf :many
SELECT subject_id, rank, period_start
FROM call_board_entries
WHERE board = sqlc.arg(board) AND period = sqlc.arg(period) AND rank IS NOT NULL
ORDER BY rank, subject_id;

-- name: LeaderCauseSince :one
SELECT proposal_id, kind, symbol, is_right
FROM (
  SELECT c.proposal_id, c.kind, c.symbol, cv.is_right, c.resolved_at
  FROM call_voters AS cv
  JOIN calls AS c ON c.proposal_id = cv.proposal_id
  WHERE cv.user_id = sqlc.arg(subject_id)::uuid
    AND cv.is_right IS NOT NULL
    AND c.state = 'resolved'
    AND c.resolved_at >= sqlc.arg(window_start)::timestamptz
    AND c.resolved_at < sqlc.arg(window_end)::timestamptz
    AND c.updated_at > coalesce(
      (SELECT started_at FROM call_board_runs ORDER BY finished_at DESC, run_id DESC LIMIT 1), '-infinity'::timestamptz
    )
  UNION ALL
  SELECT c.proposal_id, c.kind, c.symbol, cc.is_right, c.resolved_at
  FROM cabal_calls AS cc
  JOIN calls AS c ON c.proposal_id = cc.proposal_id
  WHERE cc.cabal_id = sqlc.arg(subject_id)::uuid
    AND cc.is_right IS NOT NULL
    AND c.state = 'resolved'
    AND c.resolved_at >= sqlc.arg(window_start)::timestamptz
    AND c.resolved_at < sqlc.arg(window_end)::timestamptz
    AND c.updated_at > coalesce(
      (SELECT started_at FROM call_board_runs ORDER BY finished_at DESC, run_id DESC LIMIT 1), '-infinity'::timestamptz
    )
) AS recent
ORDER BY resolved_at DESC, proposal_id DESC
LIMIT 1;

-- name: InsertLeadChange :exec
INSERT INTO call_board_lead_changes (
  id, board, period, period_start, leader_id, passed_id, leader_rate_bps, leader_prior_rank, movement_places, call,
  changed_at
) VALUES (
  sqlc.arg(id), sqlc.arg(board), sqlc.arg(period),
  (sqlc.arg(period_start)::timestamptz AT TIME ZONE 'UTC')::date, sqlc.arg(leader_id), sqlc.arg(passed_id),
  sqlc.arg(leader_rate_bps), nullif(sqlc.arg(leader_prior_rank)::int, 0), sqlc.arg(movement_places), sqlc.arg(call),
  sqlc.arg(changed_at)
)
ON CONFLICT (board, period, period_start, leader_id, changed_at) DO NOTHING;

-- name: LatestLeadChange :one
SELECT l.id, l.leader_id, l.passed_id, l.leader_rate_bps, l.movement_places, l.call, l.changed_at,
  lead.subject_kind AS leader_kind, lead.subject_name AS leader_name, lead.subject_handle AS leader_handle,
  lead.subject_picture_url AS leader_picture_url,
  pass.subject_kind AS passed_kind, pass.subject_name AS passed_name, pass.subject_handle AS passed_handle,
  pass.subject_picture_url AS passed_picture_url
FROM call_board_lead_changes AS l
JOIN call_board_entries AS lead
  ON lead.board = l.board AND lead.period = l.period AND lead.subject_id = l.leader_id
JOIN call_board_entries AS pass
  ON pass.board = l.board AND pass.period = l.period AND pass.subject_id = l.passed_id
WHERE l.board = sqlc.arg(board) AND l.period = sqlc.arg(period)
  AND l.period_start = (sqlc.arg(period_start)::timestamptz AT TIME ZONE 'UTC')::date
  AND l.changed_at > sqlc.arg(since)::timestamptz
ORDER BY l.changed_at DESC, l.id DESC
LIMIT 1;
