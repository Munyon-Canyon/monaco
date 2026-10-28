-- name: UpsertCallOpened :exec
INSERT INTO calls (proposal_id, cabal_id, kind, symbol, mint, asset_id, opened_at, state, updated_at)
VALUES (
  sqlc.arg(proposal_id), sqlc.arg(cabal_id), sqlc.arg(kind), sqlc.arg(symbol), sqlc.arg(mint),
  sqlc.narg(asset_id), sqlc.arg(opened_at)::timestamptz, 'open', sqlc.arg(at)
)
ON CONFLICT (proposal_id) DO UPDATE
SET kind = EXCLUDED.kind,
  symbol = EXCLUDED.symbol,
  mint = EXCLUDED.mint,
  asset_id = coalesce(EXCLUDED.asset_id, calls.asset_id),
  opened_at = EXCLUDED.opened_at,
  updated_at = EXCLUDED.updated_at;

-- name: UpsertCallVoter :exec
INSERT INTO call_voters (proposal_id, user_id, cabal_id, choice, choice_at)
VALUES (
  sqlc.arg(proposal_id), sqlc.arg(user_id), sqlc.arg(cabal_id), sqlc.narg(choice), sqlc.narg(choice_at)
)
ON CONFLICT (proposal_id, user_id) DO UPDATE
SET choice = EXCLUDED.choice, choice_at = EXCLUDED.choice_at
WHERE EXCLUDED.choice_at IS NOT NULL
  AND (call_voters.choice_at IS NULL OR call_voters.choice_at < EXCLUDED.choice_at);

-- name: RecordCallDecision :execrows
INSERT INTO calls (
  proposal_id, cabal_id, kind, symbol, mint, asset_id, decided_at, outcome,
  market_ref_micros, market_ref_observed_at, resolve_at, state, updated_at
)
VALUES (
  sqlc.arg(proposal_id), sqlc.arg(cabal_id), sqlc.narg(kind), sqlc.narg(symbol), sqlc.narg(mint),
  sqlc.narg(asset_id), sqlc.arg(decided_at)::timestamptz, sqlc.arg(outcome)::text, sqlc.narg(market_ref_micros),
  sqlc.narg(market_ref_observed_at), sqlc.arg(decided_at)::timestamptz + interval '24 hours', 'decided', sqlc.arg(at)
)
ON CONFLICT (proposal_id) DO UPDATE
SET kind = coalesce(calls.kind, EXCLUDED.kind),
  symbol = coalesce(calls.symbol, EXCLUDED.symbol),
  mint = coalesce(calls.mint, EXCLUDED.mint),
  asset_id = coalesce(calls.asset_id, EXCLUDED.asset_id),
  decided_at = EXCLUDED.decided_at,
  outcome = EXCLUDED.outcome,
  market_ref_micros = EXCLUDED.market_ref_micros,
  market_ref_observed_at = EXCLUDED.market_ref_observed_at,
  resolve_at = EXCLUDED.resolve_at,
  state = CASE WHEN calls.state = 'void' THEN 'void' ELSE 'decided' END,
  updated_at = EXCLUDED.updated_at
WHERE calls.decided_at IS NULL;

-- name: RecordCallFill :exec
INSERT INTO calls (proposal_id, cabal_id, fill_price_micros, filled_at, state, updated_at)
VALUES (sqlc.arg(proposal_id), sqlc.arg(cabal_id), sqlc.arg(fill_price_micros)::bigint, sqlc.arg(filled_at)::timestamptz, 'open', sqlc.arg(at))
ON CONFLICT (proposal_id) DO UPDATE
SET fill_price_micros = EXCLUDED.fill_price_micros,
  filled_at = EXCLUDED.filled_at,
  updated_at = EXCLUDED.updated_at
WHERE calls.state <> 'resolved'
  AND (
    calls.filled_at IS NULL
    OR EXCLUDED.filled_at > calls.filled_at
    OR (EXCLUDED.filled_at = calls.filled_at AND EXCLUDED.fill_price_micros > calls.fill_price_micros)
  );

-- name: VoidCall :exec
INSERT INTO calls (proposal_id, cabal_id, state, void_reason, updated_at)
VALUES (sqlc.arg(proposal_id), sqlc.arg(cabal_id), 'void', sqlc.arg(reason)::text, sqlc.arg(at))
ON CONFLICT (proposal_id) DO UPDATE
SET state = 'void', void_reason = EXCLUDED.void_reason, updated_at = EXCLUDED.updated_at
WHERE calls.state <> 'resolved';

-- name: UpsertCabalCall :exec
INSERT INTO cabal_calls (proposal_id, cabal_id, side)
VALUES (sqlc.arg(proposal_id), sqlc.arg(cabal_id), sqlc.arg(side))
ON CONFLICT (proposal_id) DO NOTHING;

-- name: ResetCallVoterChoice :exec
UPDATE call_voters
SET choice = NULL, choice_at = sqlc.arg(at)::timestamptz
WHERE proposal_id = sqlc.arg(proposal_id)
  AND user_id = sqlc.arg(user_id)
  AND (choice_at IS NULL OR choice_at < sqlc.arg(at)::timestamptz);

-- name: CallAssetOfProposal :one
SELECT asset_id FROM calls WHERE proposal_id = sqlc.arg(proposal_id);

-- name: UserCallRecords :many
WITH resolved AS (
  SELECT cv.user_id,
    count(*) FILTER (WHERE cv.is_right) AS right_calls,
    count(*) AS resolved_calls
  FROM call_voters AS cv
  JOIN calls AS c ON c.proposal_id = cv.proposal_id
  WHERE cv.user_id = ANY(sqlc.arg(user_ids)::uuid[])
    AND (sqlc.narg(cabal_id)::uuid IS NULL OR cv.cabal_id = sqlc.narg(cabal_id)::uuid)
    AND c.state = 'resolved'
    AND c.resolved_at >= sqlc.arg(window_start)::timestamptz
    AND c.resolved_at < sqlc.arg(window_end)::timestamptz
    AND cv.is_right IS NOT NULL
  GROUP BY cv.user_id
),
turnout AS (
  SELECT cv.user_id,
    count(*) FILTER (WHERE cv.choice IS NOT NULL) AS voted_calls,
    count(*) AS eligible_calls
  FROM call_voters AS cv
  JOIN calls AS c ON c.proposal_id = cv.proposal_id
  WHERE cv.user_id = ANY(sqlc.arg(user_ids)::uuid[])
    AND (sqlc.narg(cabal_id)::uuid IS NULL OR cv.cabal_id = sqlc.narg(cabal_id)::uuid)
    AND c.state <> 'void'
    AND c.kind IS NOT NULL
    AND c.opened_at >= sqlc.arg(window_start)::timestamptz
    AND c.opened_at < sqlc.arg(window_end)::timestamptz
  GROUP BY cv.user_id
)
SELECT u.user_id::uuid AS user_id,
  coalesce(r.right_calls, 0)::bigint AS right_calls,
  coalesce(r.resolved_calls, 0)::bigint AS resolved_calls,
  coalesce(t.voted_calls, 0)::bigint AS voted_calls,
  coalesce(t.eligible_calls, 0)::bigint AS eligible_calls
FROM unnest(sqlc.arg(user_ids)::uuid[]) AS u(user_id)
LEFT JOIN resolved AS r ON r.user_id = u.user_id
LEFT JOIN turnout AS t ON t.user_id = u.user_id;

-- name: UserPendingCallSymbols :many
SELECT cv.user_id, c.symbol
FROM call_voters AS cv
JOIN calls AS c ON c.proposal_id = cv.proposal_id
WHERE cv.user_id = ANY(sqlc.arg(user_ids)::uuid[])
  AND (sqlc.narg(cabal_id)::uuid IS NULL OR cv.cabal_id = sqlc.narg(cabal_id)::uuid)
  AND cv.choice IS NOT NULL
  AND c.state IN ('open', 'decided')
  AND c.symbol IS NOT NULL
  AND c.opened_at >= sqlc.arg(window_start)::timestamptz
  AND c.opened_at < sqlc.arg(window_end)::timestamptz
ORDER BY cv.user_id, c.opened_at, c.proposal_id;

-- name: CabalCallRecords :many
SELECT cc.cabal_id,
  count(*) FILTER (WHERE cc.is_right)::bigint AS right_calls,
  count(*)::bigint AS resolved_calls
FROM cabal_calls AS cc
JOIN calls AS c ON c.proposal_id = cc.proposal_id
WHERE cc.cabal_id = ANY(sqlc.arg(cabal_ids)::uuid[])
  AND c.state = 'resolved'
  AND c.resolved_at >= sqlc.arg(window_start)::timestamptz
  AND c.resolved_at < sqlc.arg(window_end)::timestamptz
  AND cc.is_right IS NOT NULL
GROUP BY cc.cabal_id;

-- name: DueCalls :many
SELECT proposal_id, cabal_id, kind, symbol, mint, asset_id, opened_at, decided_at, outcome,
  market_ref_micros, market_ref_observed_at, fill_price_micros, filled_at, ref_price_micros, ref_source,
  resolve_at, resolved_at, end_price_micros, move_bps, void_reason, state, updated_at
FROM calls
WHERE state = 'decided' AND resolve_at <= sqlc.arg(now)::timestamptz
ORDER BY resolve_at, proposal_id
LIMIT sqlc.arg(n)::int
FOR UPDATE SKIP LOCKED;

-- name: ResolveCall :execrows
UPDATE calls
SET ref_price_micros = sqlc.arg(ref_price_micros)::bigint,
  ref_source = sqlc.arg(ref_source)::text,
  end_price_micros = sqlc.arg(end_price_micros)::bigint,
  move_bps = sqlc.arg(move_bps)::int,
  resolved_at = sqlc.arg(resolved_at)::timestamptz,
  state = 'resolved',
  updated_at = sqlc.arg(at)::timestamptz
WHERE proposal_id = sqlc.arg(proposal_id) AND state = 'decided';

-- name: SetVoterRight :many
UPDATE call_voters
SET is_right = (choice = 'yes') = sqlc.arg(yes_right)::boolean
WHERE proposal_id = sqlc.arg(proposal_id) AND choice IS NOT NULL
RETURNING user_id, coalesce(is_right, false)::boolean AS is_right;

-- name: SetCabalCallRight :execrows
UPDATE cabal_calls
SET is_right = (side = 'yes') = sqlc.arg(yes_right)::boolean
WHERE proposal_id = sqlc.arg(proposal_id);

-- name: VoidUnpriced :execrows
UPDATE calls
SET state = 'void', void_reason = 'unpriced', updated_at = sqlc.arg(at)::timestamptz
WHERE proposal_id = sqlc.arg(proposal_id) AND state = 'decided';
