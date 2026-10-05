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
)
ON CONFLICT (id) DO NOTHING;

-- name: InsertUserEntry :exec
INSERT INTO user_txn_entries (txn_id, seq, account, asset, amount)
VALUES (sqlc.arg(txn_id)::uuid, sqlc.arg(seq)::smallint, sqlc.arg(account)::text, sqlc.arg(asset)::text,
  sqlc.arg(amount)::bigint);

-- name: OwnsSignature :one
SELECT EXISTS (SELECT 1 FROM cabal_txns WHERE tx_signature = sqlc.arg(tx_signature)::text)
  OR EXISTS (SELECT 1 FROM user_txns WHERE tx_signature = sqlc.arg(tx_signature)::text)
  OR EXISTS (SELECT 1 FROM fund_transfers WHERE tx_signature = sqlc.arg(tx_signature)::text);

-- name: WalletLedgerMicros :one
SELECT coalesce((SELECT sum(e.amount) FROM user_txns t JOIN user_txn_entries e ON e.txn_id = t.id
  WHERE t.user_id = sqlc.arg(user_id)::uuid AND t.status = 'settled' AND e.account = 'wallet'
    AND e.asset = sqlc.arg(asset)::text), 0)::text AS settled,
  (SELECT count(*) FROM user_txns WHERE user_id = sqlc.arg(user_id)::uuid AND status = 'pending')::bigint AS pending;

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

-- name: CabalPositions :many
SELECT asset, units::text AS units, cost_basis_micros::text AS cost_basis_micros,
  (SELECT coalesce(sum(payout_micros), 0)::text
   FROM cash_out_jobs
   WHERE cabal_id = sqlc.arg(cabal_id)::uuid
     AND status IN ('started', 'selling', 'paying')) AS cash_out_reserved_micros
FROM cabal_positions
WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND units > 0
ORDER BY asset;

-- name: CabalTotalShares :one
SELECT coalesce(sum(share_units), 0)::text AS share_units
FROM user_positions
WHERE cabal_id = sqlc.arg(cabal_id)::uuid;

-- name: CabalUserPosition :one
SELECT share_units::text AS share_units, contributed_micros::text AS contributed_micros,
  withdrawn_micros::text AS withdrawn_micros
FROM user_positions
WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND user_id = sqlc.arg(user_id)::uuid;

-- name: UserStakes :many
SELECT p.cabal_id, p.user_id, p.share_units::text AS share_units, p.contributed_micros::text AS contributed_micros,
  p.withdrawn_micros::text AS withdrawn_micros,
  coalesce((SELECT sum(all_positions.share_units) FROM user_positions AS all_positions
    WHERE all_positions.cabal_id = p.cabal_id), 0)::text AS total_shares
FROM user_positions AS p
WHERE p.user_id = sqlc.arg(user_id)::uuid AND p.share_units > 0
ORDER BY p.cabal_id;

-- name: CabalStakeSnapshot :many
WITH stake AS (
  SELECT share_units, contributed_micros, withdrawn_micros
  FROM user_positions
  WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND user_id = sqlc.arg(user_id)::uuid
), total AS (
  SELECT coalesce(sum(share_units), 0)::text AS share_units FROM user_positions
  WHERE cabal_id = sqlc.arg(cabal_id)::uuid
)
SELECT stake.share_units::text AS share_units, stake.contributed_micros::text AS contributed_micros,
  stake.withdrawn_micros::text AS withdrawn_micros, total.share_units AS total_shares,
  positions.asset, coalesce(positions.units, 0)::text AS units,
  coalesce(positions.cost_basis_micros, 0)::text AS cost_basis_micros
FROM stake CROSS JOIN total
LEFT JOIN cabal_positions AS positions ON positions.cabal_id = sqlc.arg(cabal_id)::uuid AND positions.units > 0
ORDER BY positions.asset;

-- name: CabalPositionSnapshotsAt :many
WITH RECURSIVE txn_usdc_paid AS (
  SELECT txn_id, -sum(amount)::numeric AS paid
  FROM cabal_txn_entries
  WHERE account = 'treasury' AND asset = sqlc.arg(usdc)::text
  GROUP BY txn_id
), txn_positions AS (
  SELECT t.cabal_id, t.seq, e.asset, sum(e.amount)::numeric AS delta,
    coalesce(max(paid.paid), 0)::numeric AS paid
  FROM cabal_txns AS t
  JOIN cabal_txn_entries AS e ON e.txn_id = t.id
  LEFT JOIN txn_usdc_paid AS paid ON paid.txn_id = t.id
  WHERE t.created_at <= sqlc.arg(at)::timestamptz AND e.account = 'treasury'
  GROUP BY t.id, t.cabal_id, t.seq, e.asset
  HAVING sum(e.amount) <> 0
), ordered_positions AS (
  SELECT cabal_id, seq, asset, delta, paid,
    row_number() OVER (PARTITION BY cabal_id, asset ORDER BY seq) AS ordinal
  FROM txn_positions
), replayed_positions AS (
  SELECT cabal_id, asset, ordinal, delta AS units,
    CASE
      WHEN delta < 0 THEN 0::numeric
      WHEN asset = sqlc.arg(usdc)::text THEN delta
      ELSE greatest(paid, 0)
    END AS cost_basis_micros
  FROM ordered_positions
  WHERE ordinal = 1

  UNION ALL

  SELECT next.cabal_id, next.asset, next.ordinal, replayed.units + next.delta,
    CASE
      WHEN next.delta < 0 THEN replayed.cost_basis_micros - div(
        replayed.cost_basis_micros * -next.delta, greatest(replayed.units, 1))
      WHEN next.asset = sqlc.arg(usdc)::text THEN replayed.cost_basis_micros + next.delta
      ELSE replayed.cost_basis_micros + greatest(next.paid, 0)
    END
  FROM replayed_positions AS replayed
  JOIN ordered_positions AS next
    ON next.cabal_id = replayed.cabal_id AND next.asset = replayed.asset
    AND next.ordinal = replayed.ordinal + 1
), latest_positions AS (
  SELECT DISTINCT ON (cabal_id, asset) cabal_id, asset, units, cost_basis_micros
  FROM replayed_positions
  ORDER BY cabal_id, asset, ordinal DESC
), share_totals AS (
  SELECT t.cabal_id, sum(e.amount)::text AS share_units
  FROM user_txns AS t
  JOIN user_txn_entries AS e ON e.txn_id = t.id
  WHERE t.created_at <= sqlc.arg(at)::timestamptz AND e.account = 'holder'
    AND e.asset = 'shares:' || t.cabal_id::text
  GROUP BY t.cabal_id
  HAVING sum(e.amount) > 0
)
SELECT positions.cabal_id, positions.asset, positions.units::text AS units,
  positions.cost_basis_micros::text AS cost_basis_micros, totals.share_units
FROM latest_positions AS positions
JOIN share_totals AS totals ON totals.cabal_id = positions.cabal_id
WHERE positions.units > 0
ORDER BY positions.cabal_id, positions.asset;

-- name: CabalUserShareUnitsAt :one
SELECT coalesce(sum(e.amount), 0)::text AS share_units
FROM user_txns AS t
JOIN user_txn_entries AS e ON e.txn_id = t.id
WHERE t.created_at <= sqlc.arg(at)::timestamptz AND t.cabal_id = sqlc.arg(cabal_id)::uuid
  AND t.user_id = sqlc.arg(user_id)::uuid AND e.account = 'holder' AND e.asset = 'shares:' || t.cabal_id::text;

-- name: MemberStakesAt :many
SELECT t.user_id, coalesce(t.cabal_id, '00000000-0000-0000-0000-000000000000'::uuid) AS cabal_id,
  sum(CASE WHEN e.account = 'holder' AND e.asset = 'shares:' || t.cabal_id::text
  THEN e.amount ELSE 0 END)::text AS share_units,
  sum(CASE WHEN e.account = 'cabal' THEN e.amount ELSE 0 END)::text AS net_contributed_micros
FROM user_txns AS t
JOIN user_txn_entries AS e ON e.txn_id = t.id
	WHERE t.created_at <= sqlc.arg(at)::timestamptz AND t.cabal_id IS NOT NULL
	GROUP BY t.user_id, t.cabal_id
	ORDER BY t.user_id, t.cabal_id;

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
