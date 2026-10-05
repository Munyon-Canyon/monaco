-- name: RenameLeaderboardSubject :execrows
UPDATE leaderboard_entries
SET subject_name = sqlc.arg(name), subject_handle = sqlc.narg(handle), subject_picture_url = sqlc.narg(picture_url)
WHERE subject_id = sqlc.arg(subject_id)
  AND (board = 'people' OR board LIKE 'cabal_members:%')
  AND (
    subject_name IS DISTINCT FROM sqlc.arg(name)
    OR subject_handle IS DISTINCT FROM sqlc.narg(handle)
    OR subject_picture_url IS DISTINCT FROM sqlc.narg(picture_url)
  );

-- name: BumpLatestRunRev :one
UPDATE leaderboard_runs SET rev = rev + 1
WHERE run_id = (SELECT latest.run_id FROM leaderboard_runs latest ORDER BY latest.finished_at DESC, latest.run_id DESC LIMIT 1)
RETURNING run_id, finished_at;
