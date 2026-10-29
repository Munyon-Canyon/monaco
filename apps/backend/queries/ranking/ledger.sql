-- name: InsertLedger :execrows
INSERT INTO weekly_ledgers (user_id, week, window_start, window_end, payload, built_at)
VALUES (
  sqlc.arg(user_id), sqlc.arg(week), sqlc.arg(window_start)::timestamptz, sqlc.arg(window_end)::timestamptz,
  sqlc.arg(payload)::jsonb, sqlc.arg(built_at)::timestamptz
)
ON CONFLICT (user_id, week) DO NOTHING;

-- name: InsertLedgerSkip :exec
INSERT INTO weekly_ledger_skips (user_id, week)
VALUES (sqlc.arg(user_id), sqlc.arg(week))
ON CONFLICT (user_id, week) DO NOTHING;

-- name: GetLedger :one
SELECT payload FROM weekly_ledgers WHERE user_id = sqlc.arg(user_id) AND week = sqlc.arg(week);

-- name: LatestLedger :one
SELECT payload FROM weekly_ledgers WHERE user_id = sqlc.arg(user_id) ORDER BY window_end DESC LIMIT 1;

-- name: UserCallsResolvedBetween :many
SELECT c.proposal_id, c.cabal_id, c.kind, c.symbol, c.mint, cv.choice, cv.is_right, c.move_bps, c.decided_at
FROM call_voters AS cv
JOIN calls AS c ON c.proposal_id = cv.proposal_id
WHERE cv.user_id = sqlc.arg(user_id)
  AND cv.choice IS NOT NULL
  AND cv.is_right IS NOT NULL
  AND c.state = 'resolved'
  AND c.kind IS NOT NULL
  AND c.move_bps IS NOT NULL
  AND c.decided_at IS NOT NULL
  AND c.resolved_at >= sqlc.arg(window_start)::timestamptz
  AND c.resolved_at < sqlc.arg(window_end)::timestamptz
ORDER BY c.resolved_at DESC, c.proposal_id DESC;

-- name: UserTurnoutBetween :one
SELECT
  (count(*) FILTER (WHERE cv.choice IS NOT NULL))::int AS voted,
  count(*)::int AS eligible
FROM call_voters AS cv
JOIN calls AS c ON c.proposal_id = cv.proposal_id
WHERE cv.user_id = sqlc.arg(user_id)
  AND c.state <> 'void'
  AND c.kind IS NOT NULL
  AND c.opened_at >= sqlc.arg(window_start)::timestamptz
  AND c.opened_at < sqlc.arg(window_end)::timestamptz;

-- name: CabalmatesRecordsBetween :many
SELECT cv.user_id::uuid AS user_id,
  (count(*) FILTER (WHERE cv.is_right))::int AS right_calls,
  count(*)::int AS resolved_calls
FROM call_voters AS cv
JOIN calls AS c ON c.proposal_id = cv.proposal_id
WHERE cv.cabal_id = ANY(sqlc.arg(cabal_ids)::uuid[])
  AND c.state = 'resolved'
  AND cv.is_right IS NOT NULL
  AND c.resolved_at >= sqlc.arg(window_start)::timestamptz
  AND c.resolved_at < sqlc.arg(window_end)::timestamptz
GROUP BY cv.user_id;

-- name: CallProposalsInCabals :many
SELECT c.proposal_id, c.kind, c.symbol
FROM calls AS c
WHERE c.cabal_id = ANY(sqlc.arg(cabal_ids)::uuid[])
  AND c.state = 'resolved'
  AND c.resolved_at >= sqlc.arg(window_start)::timestamptz
  AND c.resolved_at < sqlc.arg(window_end)::timestamptz;
