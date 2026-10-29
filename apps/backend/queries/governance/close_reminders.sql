-- name: UpsertReminder :exec
INSERT INTO proposal_close_reminders (proposal_id, user_id, due_at, created_at)
VALUES (@proposal_id, @user_id, @due_at, @created_at)
ON CONFLICT (proposal_id, user_id) DO UPDATE SET due_at = excluded.due_at, sent_at = NULL;

-- name: DeleteReminder :execrows
DELETE FROM proposal_close_reminders WHERE proposal_id = @proposal_id AND user_id = @user_id;

-- name: ProposalExists :one
SELECT EXISTS (SELECT 1 FROM proposals WHERE id = @id);
