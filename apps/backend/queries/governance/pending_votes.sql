-- name: PendingVotes :many
SELECT p.id, p.cabal_id, p.kind, p.symbol, p.expires_at
FROM proposal_voters AS pv
JOIN proposals AS p ON p.id = pv.proposal_id
WHERE pv.voter_id = @voter_id
  AND p.status = 'open'
  AND NOT EXISTS (SELECT 1 FROM votes AS v WHERE v.proposal_id = p.id AND v.voter_id = @voter_id)
ORDER BY p.expires_at, p.id
LIMIT 50;
