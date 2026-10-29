-- name: PeopleCallInputs :many
WITH turnout AS (
  SELECT cv.user_id,
    count(*) FILTER (WHERE cv.choice IS NOT NULL)::int AS voted,
    count(*)::int AS eligible
  FROM call_voters AS cv
  JOIN calls AS c ON c.proposal_id = cv.proposal_id
  WHERE c.state <> 'void'
    AND c.kind IS NOT NULL
    AND c.opened_at >= sqlc.arg(window_start)::timestamptz
    AND c.opened_at < sqlc.arg(window_end)::timestamptz
  GROUP BY cv.user_id
),
resolved AS (
  SELECT cv.user_id,
    count(*) FILTER (WHERE cv.is_right)::int AS right_count,
    count(*)::int AS resolved
  FROM call_voters AS cv
  JOIN calls AS c ON c.proposal_id = cv.proposal_id
  WHERE c.state = 'resolved'
    AND cv.is_right IS NOT NULL
    AND c.resolved_at >= sqlc.arg(window_start)::timestamptz
    AND c.resolved_at < sqlc.arg(window_end)::timestamptz
  GROUP BY cv.user_id
)
SELECT t.user_id::uuid AS subject_id,
  coalesce(r.right_count, 0)::int AS right_count,
  coalesce(r.resolved, 0)::int AS resolved,
  t.voted,
  t.eligible
FROM turnout AS t
LEFT JOIN resolved AS r ON r.user_id = t.user_id
ORDER BY t.user_id;

-- name: CabalCallInputs :many
WITH decided AS (
  SELECT DISTINCT cc.cabal_id
  FROM cabal_calls AS cc
  JOIN calls AS c ON c.proposal_id = cc.proposal_id
  WHERE c.state <> 'void'
    AND c.decided_at >= sqlc.arg(window_start)::timestamptz
    AND c.decided_at < sqlc.arg(window_end)::timestamptz
),
resolved AS (
  SELECT cc.cabal_id,
    count(*) FILTER (WHERE cc.is_right)::int AS right_count,
    count(*)::int AS resolved
  FROM cabal_calls AS cc
  JOIN calls AS c ON c.proposal_id = cc.proposal_id
  WHERE c.state = 'resolved'
    AND cc.is_right IS NOT NULL
    AND c.resolved_at >= sqlc.arg(window_start)::timestamptz
    AND c.resolved_at < sqlc.arg(window_end)::timestamptz
  GROUP BY cc.cabal_id
),
turnout AS (
  SELECT cv.cabal_id,
    count(*) FILTER (WHERE cv.choice IS NOT NULL)::int AS voted,
    count(*)::int AS eligible
  FROM call_voters AS cv
  JOIN calls AS c ON c.proposal_id = cv.proposal_id
  WHERE c.state <> 'void'
    AND c.kind IS NOT NULL
    AND c.opened_at >= sqlc.arg(window_start)::timestamptz
    AND c.opened_at < sqlc.arg(window_end)::timestamptz
  GROUP BY cv.cabal_id
)
SELECT d.cabal_id::uuid AS subject_id,
  coalesce(r.right_count, 0)::int AS right_count,
  coalesce(r.resolved, 0)::int AS resolved,
  coalesce(t.voted, 0)::int AS voted,
  coalesce(t.eligible, 0)::int AS eligible
FROM decided AS d
LEFT JOIN resolved AS r ON r.cabal_id = d.cabal_id
LEFT JOIN turnout AS t ON t.cabal_id = d.cabal_id
ORDER BY d.cabal_id;

-- name: DeleteCallBoardEntries :exec
DELETE FROM call_board_entries WHERE board = sqlc.arg(board) AND period = sqlc.arg(period);

-- name: InsertCallBoardEntries :exec
INSERT INTO call_board_entries (
  board, period, period_start, subject_id, subject_kind, subject_name, subject_handle, subject_picture_url,
  subject_created_at, rank, right_count, resolved, voted, eligible, rate_bps, turnout_bps, baseline_rank, movement,
  movement_kind, last_call, run_id, computed_at
)
SELECT board, period, period_start, subject_id, subject_kind, subject_name, subject_handle, subject_picture_url,
  subject_created_at, rank, right_count, resolved, voted, eligible, rate_bps, turnout_bps, baseline_rank, movement,
  movement_kind, last_call, run_id, computed_at
FROM jsonb_to_recordset(sqlc.arg(rows)::jsonb) AS rows(
  board text, period text, period_start date, subject_id uuid, subject_kind text, subject_name text,
  subject_handle text, subject_picture_url text, subject_created_at timestamptz, rank int, right_count int,
  resolved int, voted int, eligible int, rate_bps int, turnout_bps int, baseline_rank int, movement int,
  movement_kind text, last_call jsonb, run_id uuid, computed_at timestamptz
);

-- name: InsertCallBoardTrigger :exec
INSERT INTO call_board_triggers (reason, created_at)
VALUES (sqlc.arg(reason), sqlc.arg(created_at));

-- name: OldestCallBoardTrigger :one
SELECT created_at FROM call_board_triggers ORDER BY created_at, id LIMIT 1;

-- name: DeleteCallBoardTriggersThrough :exec
DELETE FROM call_board_triggers WHERE created_at <= sqlc.arg(started_at);

-- name: InsertCallBoardRun :exec
INSERT INTO call_board_runs (run_id, started_at, finished_at, rows_written)
SELECT sqlc.arg(run_id)::uuid, sqlc.arg(started_at)::timestamptz, sqlc.arg(finished_at)::timestamptz, count(*)::int
FROM call_board_entries
WHERE run_id = sqlc.arg(run_id)::uuid;

-- name: CallBoardLastRun :one
SELECT run_id, started_at, finished_at, rows_written
FROM call_board_runs
ORDER BY finished_at DESC, run_id DESC
LIMIT 1;

-- name: CallStandingOf :one
SELECT e.rank, e.right_count, e.resolved, e.rate_bps, e.period_start,
  (
    SELECT count(*) FROM call_board_entries AS r
    WHERE r.board = 'people' AND r.period = e.period AND r.rank IS NOT NULL
  )::int AS ranked
FROM call_board_entries AS e
WHERE e.board = 'people' AND e.period = sqlc.arg(period) AND e.subject_id = sqlc.arg(user_id);

-- name: CallLeaderOf :one
SELECT subject_id, subject_name, subject_handle, subject_picture_url, right_count, resolved, rate_bps, period_start
FROM call_board_entries
WHERE board = sqlc.arg(board) AND period = sqlc.arg(period) AND rank = 1;

-- name: CallBoardRanSince :one
SELECT EXISTS (SELECT 1 FROM call_board_runs WHERE started_at >= sqlc.arg(since)::timestamptz)::boolean AS ran;

-- name: UpsertBaselines :exec
INSERT INTO call_board_baselines (board, period, period_start, day, subject_id, rank)
SELECT sqlc.arg(board)::text, sqlc.arg(period)::text, (sqlc.arg(period_start)::timestamptz AT TIME ZONE 'UTC')::date,
  (sqlc.arg(day)::timestamptz AT TIME ZONE 'UTC')::date,
  rows.subject_id, rows.rank
FROM jsonb_to_recordset(sqlc.arg(rows)::jsonb) AS rows(subject_id uuid, rank int)
ON CONFLICT (board, period, day, subject_id) DO NOTHING;

-- name: BaselinesFor :many
SELECT subject_id, rank
FROM call_board_baselines
WHERE board = sqlc.arg(board) AND period = sqlc.arg(period)
  AND day = (sqlc.arg(day)::timestamptz AT TIME ZONE 'UTC')::date;

-- name: PruneBaselines :exec
DELETE FROM call_board_baselines WHERE day < (sqlc.arg(before)::timestamptz AT TIME ZONE 'UTC')::date;

-- name: LastCallsSince :many
SELECT DISTINCT ON (subject_id) subject_id, proposal_id, kind, symbol, is_right AS right_call
FROM (
  SELECT cv.user_id AS subject_id, c.proposal_id, c.kind, c.symbol, cv.is_right, c.resolved_at
  FROM call_voters AS cv
  JOIN calls AS c ON c.proposal_id = cv.proposal_id
  WHERE cv.user_id = ANY(sqlc.arg(subject_ids)::uuid[])
    AND cv.is_right IS NOT NULL
    AND c.state = 'resolved'
    AND c.resolved_at >= sqlc.arg(since)::timestamptz
  UNION ALL
  SELECT cc.cabal_id AS subject_id, c.proposal_id, c.kind, c.symbol, cc.is_right, c.resolved_at
  FROM cabal_calls AS cc
  JOIN calls AS c ON c.proposal_id = cc.proposal_id
  WHERE cc.cabal_id = ANY(sqlc.arg(subject_ids)::uuid[])
    AND cc.is_right IS NOT NULL
    AND c.state = 'resolved'
    AND c.resolved_at >= sqlc.arg(since)::timestamptz
) AS resolved_calls
ORDER BY subject_id, resolved_at DESC, proposal_id DESC;

-- name: ClosedPeriodsPending :many
WITH stamps AS (
  SELECT unnest(ARRAY[opened_at, decided_at, resolved_at]) AT TIME ZONE 'UTC' AS ts
  FROM calls
  WHERE state <> 'void'
),
periods AS (
  SELECT DISTINCT 'week'::text AS period, date_trunc('week', ts)::date AS period_start FROM stamps WHERE ts IS NOT NULL
  UNION
  SELECT DISTINCT 'month'::text, date_trunc('month', ts)::date FROM stamps WHERE ts IS NOT NULL
)
SELECT p.period, p.period_start
FROM periods AS p
WHERE (p.period_start + CASE p.period WHEN 'week' THEN interval '7 days' ELSE interval '1 month' END)
    <= (sqlc.arg(ended_before)::timestamptz AT TIME ZONE 'UTC')
  AND NOT EXISTS (
    SELECT 1 FROM call_board_closed AS k WHERE k.period = p.period AND k.period_start = p.period_start
  )
ORDER BY p.period_start, p.period;

-- name: InsertClosedBoard :exec
INSERT INTO call_board_closed (
  board, period, period_start, rank, subject_id, rate_bps, right_count, resolved, voted, eligible, closed_at
)
SELECT board, period, period_start, rank, subject_id, rate_bps, right_count, resolved, voted, eligible,
  sqlc.arg(closed_at)::timestamptz
FROM jsonb_to_recordset(sqlc.arg(rows)::jsonb) AS rows(
  board text, period text, period_start date, rank int, subject_id uuid, rate_bps int, right_count int,
  resolved int, voted int, eligible int
)
ON CONFLICT (board, period, period_start, subject_id) DO NOTHING;

-- name: ClosedTopsOf :many
SELECT period_start, right_count, resolved
FROM call_board_closed
WHERE subject_id = sqlc.arg(subject_id) AND board = sqlc.arg(board) AND period = sqlc.arg(period) AND rank = 1
ORDER BY period_start DESC;
