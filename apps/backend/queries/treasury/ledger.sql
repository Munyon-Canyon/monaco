-- name: LockCabalLedger :exec
SELECT pg_advisory_xact_lock(hashtextextended('cabal-ledger:' || sqlc.arg(cabal_id)::uuid::text, 0));

-- name: InsertCabalTxn :execrows
INSERT INTO cabal_txns (id, cabal_id, kind, status, swap_id, transfer_id, tx_signature, created_at, seq)
SELECT sqlc.arg(id)::uuid, sqlc.arg(cabal_id)::uuid, sqlc.arg(kind)::text, sqlc.arg(status)::text,
  NULLIF(sqlc.arg(swap_id)::uuid, '00000000-0000-0000-0000-000000000000'),
  NULLIF(sqlc.arg(transfer_id)::uuid, '00000000-0000-0000-0000-000000000000'),
  NULLIF(sqlc.arg(tx_signature)::text, ''),
  sqlc.arg(created_at)::timestamptz,
  (SELECT coalesce(max(seq), 0) + 1 FROM cabal_txns WHERE cabal_id = sqlc.arg(cabal_id)::uuid)
WHERE NOT EXISTS (
  SELECT 1 FROM cabal_txns c WHERE c.transfer_id = sqlc.arg(transfer_id)::uuid AND c.status <> sqlc.arg(status)::text
  UNION ALL
  SELECT 1 FROM user_txns u WHERE u.transfer_id = sqlc.arg(transfer_id)::uuid AND u.status <> sqlc.arg(status)::text
);

-- name: InsertCabalEntry :exec
INSERT INTO cabal_txn_entries (txn_id, seq, account, asset, amount)
VALUES (sqlc.arg(txn_id)::uuid, sqlc.arg(seq)::smallint, sqlc.arg(account)::text, sqlc.arg(asset)::text,
  sqlc.arg(amount)::bigint);

-- name: InsertUserTxn :execrows
INSERT INTO user_txns (id, user_id, cabal_id, kind, status, transfer_id, tx_signature, created_at)
SELECT sqlc.arg(id)::uuid, sqlc.arg(user_id)::uuid,
  NULLIF(sqlc.arg(cabal_id)::uuid, '00000000-0000-0000-0000-000000000000'),
  sqlc.arg(kind)::text, sqlc.arg(status)::text,
  NULLIF(sqlc.arg(transfer_id)::uuid, '00000000-0000-0000-0000-000000000000'),
  NULLIF(sqlc.arg(tx_signature)::text, ''),
  sqlc.arg(created_at)::timestamptz
WHERE NOT EXISTS (
  SELECT 1 FROM cabal_txns c WHERE c.transfer_id = sqlc.arg(transfer_id)::uuid AND c.status <> sqlc.arg(status)::text
  UNION ALL
  SELECT 1 FROM user_txns u WHERE u.transfer_id = sqlc.arg(transfer_id)::uuid AND u.status <> sqlc.arg(status)::text
);

-- name: InsertUserEntry :exec
INSERT INTO user_txn_entries (txn_id, seq, account, asset, amount)
VALUES (sqlc.arg(txn_id)::uuid, sqlc.arg(seq)::smallint, sqlc.arg(account)::text, sqlc.arg(asset)::text,
  sqlc.arg(amount)::bigint);

-- name: ApplyCabalPosition :one
WITH updated AS (
  UPDATE cabal_positions AS p SET
    units = p.units + sqlc.arg(delta)::bigint,
    cost_basis_micros = CASE
      WHEN sqlc.arg(delta)::bigint < 0
        THEN p.cost_basis_micros - div(p.cost_basis_micros * -sqlc.arg(delta)::bigint, greatest(p.units, 1))
      ELSE p.cost_basis_micros + sqlc.arg(cost_in)::text::numeric
    END,
    updated_at = sqlc.arg(updated_at)::timestamptz
  WHERE p.cabal_id = sqlc.arg(cabal_id)::uuid AND p.asset = sqlc.arg(asset)::text
  RETURNING p.units
), inserted AS (
  INSERT INTO cabal_positions (cabal_id, asset, units, cost_basis_micros, updated_at)
  SELECT sqlc.arg(cabal_id)::uuid, sqlc.arg(asset)::text, sqlc.arg(delta)::bigint, sqlc.arg(cost_in)::text::numeric,
    sqlc.arg(updated_at)::timestamptz
  WHERE NOT EXISTS (SELECT 1 FROM updated)
  RETURNING units
)
SELECT (units - sqlc.arg(delta)::bigint)::text AS before, units::text AS after
FROM (SELECT units FROM updated UNION ALL SELECT units FROM inserted) AS applied;

-- name: ApplyUserPosition :one
WITH updated AS (
  UPDATE user_positions AS p SET
    share_units = p.share_units + sqlc.arg(shares)::bigint,
    contributed_micros = p.contributed_micros + sqlc.arg(contributed)::text::numeric,
    withdrawn_micros = p.withdrawn_micros + sqlc.arg(withdrawn)::text::numeric,
    updated_at = sqlc.arg(updated_at)::timestamptz
  WHERE p.user_id = sqlc.arg(user_id)::uuid AND p.cabal_id = sqlc.arg(cabal_id)::uuid
  RETURNING p.share_units
), inserted AS (
  INSERT INTO user_positions (user_id, cabal_id, share_units, contributed_micros, withdrawn_micros, updated_at)
  SELECT sqlc.arg(user_id)::uuid, sqlc.arg(cabal_id)::uuid, sqlc.arg(shares)::bigint,
    sqlc.arg(contributed)::text::numeric, sqlc.arg(withdrawn)::text::numeric, sqlc.arg(updated_at)::timestamptz
  WHERE NOT EXISTS (SELECT 1 FROM updated)
  RETURNING share_units
)
SELECT (share_units - sqlc.arg(shares)::bigint)::text AS before, share_units::text AS after
FROM (SELECT share_units FROM updated UNION ALL SELECT share_units FROM inserted) AS applied;

-- name: SetTransferStatus :one
WITH c AS (
  UPDATE cabal_txns SET status = sqlc.arg(to_status)::text
  WHERE transfer_id = sqlc.arg(transfer_id)::uuid AND status = sqlc.arg(from_status)::text
  RETURNING 1
), u AS (
  UPDATE user_txns SET status = sqlc.arg(to_status)::text
  WHERE transfer_id = sqlc.arg(transfer_id)::uuid AND status = sqlc.arg(from_status)::text
  RETURNING 1
)
SELECT (SELECT count(*) FROM c)::bigint AS cabal_rows, (SELECT count(*) FROM u)::bigint AS user_rows;
