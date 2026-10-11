-- name: InsertSwapOperatorAction :exec
INSERT INTO swap_operator_actions (id, swap_id, action, actor, reason, detail, created_at)
VALUES (
  sqlc.arg(id)::uuid, sqlc.arg(swap_id)::uuid, sqlc.arg(action)::text, sqlc.arg(actor)::text,
  sqlc.arg(reason)::text, sqlc.arg(detail)::jsonb, sqlc.arg(created_at)::timestamptz
);
