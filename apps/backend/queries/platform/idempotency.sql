-- name: BeginIdempotencyKey :execrows
INSERT INTO idempotency_keys (actor_key, key, request_hash, status, created_at)
VALUES ($1, $2, $3, 1, $4)
ON CONFLICT (actor_key, key) DO NOTHING;

-- name: GetIdempotencyKey :one
SELECT actor_key, key, request_hash, status, response_status, response_body, response_headers, created_at, completed_at,
  claim_generation
FROM idempotency_keys
WHERE actor_key = $1 AND key = $2;

-- name: CompleteIdempotencyKey :execrows
UPDATE idempotency_keys
SET status = 2, response_status = $3, response_body = $4, response_headers = $5, completed_at = $6
WHERE actor_key = $1 AND key = $2 AND status = 1 AND claim_generation = $7;

-- name: ReleaseIdempotencyKey :execrows
DELETE FROM idempotency_keys WHERE actor_key = $1 AND key = $2 AND status = 1 AND claim_generation = $3;

-- name: DeleteIdempotencyKeysBefore :execrows
DELETE FROM idempotency_keys WHERE created_at < $1;

-- name: TakeOverIdempotencyKey :one
UPDATE idempotency_keys SET created_at = $3, claim_generation = claim_generation + 1
WHERE actor_key = $1 AND key = $2 AND status = 1 AND created_at < $4
RETURNING claim_generation;

-- name: BindMoneyIntent :execrows
INSERT INTO money_intents (actor_key, key, operation, request_hash, resource_id, created_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (actor_key, key) DO NOTHING;

-- name: GetMoneyIntent :one
SELECT operation, request_hash, resource_id
FROM money_intents
WHERE actor_key = $1 AND key = $2;

-- name: UnbindMoneyIntent :execrows
DELETE FROM money_intents WHERE operation = $1 AND resource_id = $2;
