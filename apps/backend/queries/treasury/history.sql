-- name: CabalContributionHistory :many
WITH per_txn AS (
  SELECT t.id, t.created_at, sum(e.amount) AS delta
  FROM user_txns AS t
  JOIN user_txn_entries AS e ON e.txn_id = t.id
  WHERE t.cabal_id = sqlc.arg(cabal_id)::uuid AND e.account = 'cabal'
  GROUP BY t.id, t.created_at
)
SELECT created_at, (sum(delta) OVER (ORDER BY created_at, id))::text AS net_contributed_micros
FROM per_txn
ORDER BY created_at, id;

-- name: UserStakeHistory :many
WITH per_txn AS (
  SELECT t.id, t.cabal_id, t.created_at,
    sum(CASE WHEN e.account = 'holder' AND e.asset = 'shares:' || t.cabal_id::text THEN e.amount ELSE 0 END) AS shares,
    sum(CASE WHEN e.account = 'cabal' THEN e.amount ELSE 0 END) AS contributed
  FROM user_txns AS t
  JOIN user_txn_entries AS e ON e.txn_id = t.id
  WHERE t.user_id = sqlc.arg(user_id)::uuid AND t.cabal_id IS NOT NULL
  GROUP BY t.id, t.cabal_id, t.created_at
  HAVING sum(CASE WHEN e.account = 'holder' AND e.asset = 'shares:' || t.cabal_id::text THEN e.amount ELSE 0 END) <> 0
    OR sum(CASE WHEN e.account = 'cabal' THEN e.amount ELSE 0 END) <> 0
)
SELECT cabal_id, created_at,
  (sum(shares) OVER (PARTITION BY cabal_id ORDER BY created_at, id))::text AS share_units,
  (sum(contributed) OVER (PARTITION BY cabal_id ORDER BY created_at, id))::text AS net_contributed_micros
FROM per_txn
ORDER BY created_at, id;

-- name: MemberFlowsBetween :many
SELECT t.user_id, t.cabal_id::uuid AS cabal_id, t.created_at, sum(e.amount)::text AS amount
FROM user_txns AS t
JOIN user_txn_entries AS e ON e.txn_id = t.id
WHERE t.created_at > sqlc.arg(from_at)::timestamptz AND t.created_at <= sqlc.arg(to_at)::timestamptz
  AND t.cabal_id IS NOT NULL AND e.account = 'cabal'
GROUP BY t.id, t.user_id, t.cabal_id, t.created_at
HAVING sum(e.amount) <> 0
ORDER BY t.created_at, t.id;
