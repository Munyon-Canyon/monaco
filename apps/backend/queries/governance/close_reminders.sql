-- name: UpsertReminder :exec
INSERT INTO proposal_close_reminders (proposal_id, user_id, due_at, created_at)
VALUES (@proposal_id, @user_id, @due_at, @created_at)
ON CONFLICT (proposal_id, user_id) DO UPDATE SET due_at = excluded.due_at, sent_at = NULL;

-- name: DeleteReminder :execrows
DELETE FROM proposal_close_reminders WHERE proposal_id = @proposal_id AND user_id = @user_id;

-- name: ProposalExists :one
SELECT EXISTS (SELECT 1 FROM proposals WHERE id = @id);

-- name: DueReminders :many
SELECT proposal_id, user_id FROM proposal_close_reminders
WHERE due_at <= @now AND sent_at IS NULL
ORDER BY due_at
LIMIT @batch;

-- name: LockReminder :one
SELECT due_at, sent_at FROM proposal_close_reminders
WHERE proposal_id = @proposal_id AND user_id = @user_id
FOR UPDATE;

-- name: MarkReminderSent :execrows
UPDATE proposal_close_reminders SET sent_at = @now::timestamptz
WHERE proposal_id = @proposal_id AND user_id = @user_id AND sent_at IS NULL;

-- name: HasBallot :one
SELECT EXISTS (SELECT 1 FROM votes WHERE proposal_id = @proposal_id AND voter_id = @voter_id);
