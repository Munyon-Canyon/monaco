-- name: ReferralToQualify :one
SELECT id, referrer_id FROM referrals
WHERE referee_id = sqlc.arg(referee_id) AND status = 'attributed';

-- name: QualifyReferral :execrows
UPDATE referrals SET status = 'qualified', qualified_at = sqlc.arg(qualified_at)::timestamptz
WHERE id = sqlc.arg(id) AND status = 'attributed';
