-- name: AdminUserTxns :many
SELECT id, user_id, cabal_id, kind, status, tx_signature, created_at
FROM user_txns
WHERE user_id = @user_id
  AND (NOT @has_cursor::bool OR (created_at, id) < (@cursor_at::timestamptz, @cursor_id::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @row_limit::bigint;

-- name: AdminCabalTxns :many
SELECT id, cabal_id, kind, status, tx_signature, created_at
FROM cabal_txns
WHERE cabal_id = @cabal_id
  AND (NOT @has_cursor::bool OR (created_at, id) < (@cursor_at::timestamptz, @cursor_id::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @row_limit::bigint;

-- name: AdminTxnByID :many
WITH header AS (
  SELECT u.id, 'user'::text AS scope, u.user_id::text AS user_id, u.cabal_id AS cabal_id, u.kind, u.status,
    NULL::uuid AS swap_id, u.transfer_id AS transfer_id, u.tx_signature AS tx_signature, u.created_at
  FROM user_txns AS u
  WHERE u.id = @id
  UNION ALL
  SELECT c.id, 'cabal'::text, ''::text, c.cabal_id, c.kind, c.status, c.swap_id, c.transfer_id, c.tx_signature,
    c.created_at
  FROM cabal_txns AS c
  WHERE c.id = @id
)
SELECT h.id, h.scope, h.user_id, h.cabal_id, h.kind,
  h.status, h.swap_id, h.transfer_id, h.tx_signature, h.created_at, (e.seq IS NOT NULL)::bool AS has_entry,
  coalesce(e.seq, 0)::smallint AS seq, coalesce(e.account, '')::text AS account, coalesce(e.asset, '')::text AS asset,
  coalesce(e.amount, 0)::bigint AS amount
FROM header AS h
LEFT JOIN LATERAL (
  SELECT ue.seq, ue.account, ue.asset, ue.amount FROM user_txn_entries AS ue WHERE h.scope = 'user' AND ue.txn_id = h.id
  UNION ALL
  SELECT ce.seq, ce.account, ce.asset, ce.amount FROM cabal_txn_entries AS ce WHERE h.scope = 'cabal' AND ce.txn_id = h.id
) AS e ON true
ORDER BY h.scope, e.seq;

-- name: AdminTxnBySignature :many
WITH matched AS (
  SELECT u.id, 'user'::text AS scope, u.user_id::text AS user_id, u.cabal_id AS cabal_id, u.kind, u.status,
    NULL::uuid AS swap_id, u.transfer_id AS transfer_id, u.tx_signature AS tx_signature, u.created_at
  FROM user_txns AS u
  WHERE u.tx_signature = @tx_signature::text
  UNION ALL
  SELECT c.id, 'cabal'::text, ''::text, c.cabal_id, c.kind, c.status, c.swap_id, c.transfer_id, c.tx_signature,
    c.created_at
  FROM cabal_txns AS c
  WHERE c.tx_signature = @tx_signature::text
), header AS (
  SELECT id, scope, user_id, cabal_id, kind, status, swap_id, transfer_id, tx_signature, created_at
  FROM matched ORDER BY scope, created_at, id LIMIT 1
)
SELECT h.id, h.scope, h.user_id, h.cabal_id, h.kind,
  h.status, h.swap_id, h.transfer_id, h.tx_signature, h.created_at, (e.seq IS NOT NULL)::bool AS has_entry,
  coalesce(e.seq, 0)::smallint AS seq, coalesce(e.account, '')::text AS account, coalesce(e.asset, '')::text AS asset,
  coalesce(e.amount, 0)::bigint AS amount
FROM header AS h
LEFT JOIN LATERAL (
  SELECT ue.seq, ue.account, ue.asset, ue.amount FROM user_txn_entries AS ue WHERE h.scope = 'user' AND ue.txn_id = h.id
  UNION ALL
  SELECT ce.seq, ce.account, ce.asset, ce.amount FROM cabal_txn_entries AS ce WHERE h.scope = 'cabal' AND ce.txn_id = h.id
) AS e ON true
ORDER BY h.scope, e.seq;

-- name: AdminUserShares :many
SELECT cabal_id, user_id, share_units::text AS share_units
FROM user_positions
WHERE user_id = @user_id AND share_units > 0
ORDER BY cabal_id;

-- name: AdminCabalShares :many
SELECT cabal_id, user_id, share_units::text AS share_units
FROM user_positions
WHERE cabal_id = @cabal_id AND share_units > 0
ORDER BY user_id;

-- name: AdminCabalHoldings :many
SELECT asset, units::text AS units
FROM cabal_positions
WHERE cabal_id = @cabal_id AND units > 0
ORDER BY asset;
