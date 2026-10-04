-- name: InsertLeaderboardRun :exec
INSERT INTO leaderboard_runs (
  run_id, as_of, prices_as_of, started_at, finished_at, rows_written, cabals_excluded
) VALUES (
  sqlc.arg(run_id), sqlc.arg(as_of), sqlc.arg(prices_as_of), sqlc.arg(started_at), sqlc.arg(finished_at),
  sqlc.arg(rows_written), sqlc.arg(cabals_excluded)
);

-- name: LastLeaderboardRun :one
SELECT run_id, as_of, prices_as_of, started_at, finished_at, rows_written, cabals_excluded, rev
FROM leaderboard_runs
ORDER BY finished_at DESC, run_id DESC
LIMIT 1;
