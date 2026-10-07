-- name: RefereesWithManyReferrals :many
SELECT referee_id, count(*)::bigint AS n FROM referrals
GROUP BY referee_id HAVING count(*) > 1
ORDER BY referee_id;

-- name: QualifiedReferralsWithoutProof :many
SELECT r.id, (r.qualified_at IS NOT NULL)::boolean AS has_qualified_at,
  (SELECT count(*) FROM events e WHERE e.type = 'referral.qualified' AND e.aggregate_id = r.id)::bigint AS events
FROM referrals r
WHERE r.status = 'qualified'
  AND (r.qualified_at IS NULL
    OR (SELECT count(*) FROM events e WHERE e.type = 'referral.qualified' AND e.aggregate_id = r.id) <> 1)
ORDER BY r.id;

-- name: ReferralClickColumns :many
SELECT column_name::text FROM information_schema.columns
WHERE table_schema = current_schema() AND table_name = 'referral_clicks'
ORDER BY column_name;
