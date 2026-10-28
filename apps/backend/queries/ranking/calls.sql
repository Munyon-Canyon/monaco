-- name: UpsertCallOpened :exec
INSERT INTO calls (proposal_id, cabal_id, kind, symbol, mint, asset_id, opened_at, state, updated_at)
VALUES (
  sqlc.arg(proposal_id), sqlc.arg(cabal_id), sqlc.arg(kind), sqlc.arg(symbol), sqlc.arg(mint),
  sqlc.narg(asset_id), sqlc.arg(opened_at), 'open', sqlc.arg(at)
)
ON CONFLICT (proposal_id) DO UPDATE SET
  kind = excluded.kind,
  symbol = excluded.symbol,
  mint = excluded.mint,
  asset_id = excluded.asset_id,
  opened_at = excluded.opened_at,
  updated_at = excluded.updated_at;

-- name: UpsertCallVoter :exec
INSERT INTO call_voters (proposal_id, user_id, cabal_id, choice, choice_at)
VALUES (
  sqlc.arg(proposal_id), sqlc.arg(user_id), sqlc.arg(cabal_id), sqlc.narg(choice), sqlc.narg(choice_at)
)
ON CONFLICT (proposal_id, user_id) DO UPDATE SET
  choice = excluded.choice,
  choice_at = excluded.choice_at
WHERE call_voters.choice_at IS NULL OR call_voters.choice_at < excluded.choice_at;

-- name: RecordCallDecision :execrows
INSERT INTO calls (
  proposal_id, cabal_id, decided_at, outcome, market_ref_micros, market_ref_observed_at, resolve_at, state,
  updated_at
)
VALUES (
  sqlc.arg(proposal_id), sqlc.arg(cabal_id), sqlc.arg(decided_at)::timestamptz, sqlc.arg(outcome),
  sqlc.narg(market_ref_micros), sqlc.narg(market_ref_observed_at),
  sqlc.arg(decided_at)::timestamptz + interval '24 hours', 'decided', sqlc.arg(decided_at)::timestamptz
)
ON CONFLICT (proposal_id) DO UPDATE SET
  decided_at = excluded.decided_at,
  outcome = excluded.outcome,
  market_ref_micros = excluded.market_ref_micros,
  market_ref_observed_at = excluded.market_ref_observed_at,
  resolve_at = excluded.resolve_at,
  state = CASE WHEN calls.state = 'void' THEN 'void' ELSE 'decided' END,
  updated_at = excluded.updated_at
WHERE calls.decided_at IS NULL;

-- name: RecordCabalCall :exec
INSERT INTO cabal_calls (proposal_id, cabal_id, side)
VALUES (sqlc.arg(proposal_id), sqlc.arg(cabal_id), sqlc.arg(side))
ON CONFLICT (proposal_id) DO NOTHING;

-- name: RecordCallFill :exec
INSERT INTO calls (proposal_id, cabal_id, fill_price_micros, filled_at, state, updated_at)
VALUES (
  sqlc.arg(proposal_id), sqlc.arg(cabal_id), sqlc.arg(fill_price_micros), sqlc.arg(filled_at), 'open',
  sqlc.arg(at)
)
ON CONFLICT (proposal_id) DO UPDATE SET
  fill_price_micros = excluded.fill_price_micros,
  filled_at = excluded.filled_at,
  updated_at = excluded.updated_at
WHERE calls.state <> 'resolved' AND (calls.filled_at IS NULL OR calls.filled_at < excluded.filled_at);

-- name: VoidCall :exec
INSERT INTO calls (proposal_id, cabal_id, void_reason, state, updated_at)
VALUES (sqlc.arg(proposal_id), sqlc.arg(cabal_id), sqlc.arg(reason), 'void', sqlc.arg(at))
ON CONFLICT (proposal_id) DO UPDATE SET
  state = 'void',
  void_reason = excluded.void_reason,
  updated_at = excluded.updated_at
WHERE calls.state <> 'resolved';

-- name: ResetCallVoterChoice :execrows
UPDATE call_voters
SET choice = NULL, choice_at = sqlc.arg(at)
WHERE proposal_id = sqlc.arg(proposal_id)
  AND user_id = sqlc.arg(user_id)
  AND (choice_at IS NULL OR choice_at < sqlc.arg(at));

-- name: CallAssetIDs :many
SELECT asset_id FROM calls WHERE proposal_id = sqlc.arg(proposal_id);

-- name: UserCallRecords :many
SELECT
  cv.user_id,
  (count(*) FILTER (WHERE cv."right" AND c.resolved_at >= sqlc.arg(window_start)::timestamptz
    AND c.resolved_at < sqlc.arg(window_end)::timestamptz))::int AS right_calls,
  (count(*) FILTER (WHERE cv."right" IS NOT NULL AND c.resolved_at >= sqlc.arg(window_start)::timestamptz
    AND c.resolved_at < sqlc.arg(window_end)::timestamptz))::int AS resolved,
  (count(*) FILTER (WHERE cv.choice IS NOT NULL AND c.state <> 'void' AND c.opened_at >= sqlc.arg(window_start)::timestamptz
    AND c.opened_at < sqlc.arg(window_end)::timestamptz))::int AS voted,
  (count(*) FILTER (WHERE c.state <> 'void' AND c.opened_at >= sqlc.arg(window_start)::timestamptz
    AND c.opened_at < sqlc.arg(window_end)::timestamptz))::int AS eligible
FROM call_voters AS cv
JOIN calls AS c ON c.proposal_id = cv.proposal_id
WHERE cv.user_id = ANY(sqlc.arg(user_ids)::uuid[])
  AND (coalesce(cardinality(sqlc.arg(cabal_ids)::uuid[]), 0) = 0 OR cv.cabal_id = ANY(sqlc.arg(cabal_ids)::uuid[]))
  AND c.kind IS NOT NULL
GROUP BY cv.user_id;

-- name: UserPendingCallSymbols :many
SELECT cv.user_id, c.symbol
FROM call_voters AS cv
JOIN calls AS c ON c.proposal_id = cv.proposal_id
WHERE cv.user_id = ANY(sqlc.arg(user_ids)::uuid[])
  AND (coalesce(cardinality(sqlc.arg(cabal_ids)::uuid[]), 0) = 0 OR cv.cabal_id = ANY(sqlc.arg(cabal_ids)::uuid[]))
  AND cv.choice IS NOT NULL
  AND c.kind IS NOT NULL
  AND c.state IN ('open', 'decided')
  AND c.opened_at < sqlc.arg(window_end)::timestamptz
ORDER BY cv.user_id, c.opened_at, c.proposal_id;

-- name: CabalCallRecords :many
SELECT
  c.cabal_id,
  (count(*) FILTER (WHERE cc."right"))::int AS right_calls,
  (count(*) FILTER (WHERE cc."right" IS NOT NULL))::int AS resolved
FROM calls AS c
JOIN cabal_calls AS cc ON cc.proposal_id = c.proposal_id
WHERE c.cabal_id = ANY(sqlc.arg(cabal_ids)::uuid[])
  AND c.kind IS NOT NULL
  AND c.resolved_at >= sqlc.arg(window_start)::timestamptz
  AND c.resolved_at < sqlc.arg(window_end)::timestamptz
GROUP BY c.cabal_id;
