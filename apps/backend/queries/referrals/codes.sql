-- name: MintReferralCode :one
WITH inserted AS (
  INSERT INTO referral_codes (code, user_id, created_at)
  VALUES (sqlc.arg(code), sqlc.arg(owner), sqlc.arg(created_at))
  ON CONFLICT DO NOTHING
  RETURNING code
)
SELECT (
  EXISTS (SELECT 1 FROM inserted)
  OR EXISTS (SELECT 1 FROM referral_codes rc WHERE rc.user_id = sqlc.arg(owner))
)::boolean AS minted;

-- name: ReferralCodeOwner :one
SELECT user_id FROM referral_codes WHERE code = sqlc.arg(code);

-- name: ReferralCodeOfUser :one
SELECT code FROM referral_codes WHERE user_id = sqlc.arg(owner);

-- name: ReferralAttached :one
SELECT EXISTS (SELECT 1 FROM referrals WHERE referee_id = sqlc.arg(referee_id));

-- name: InsertReferral :one
INSERT INTO referrals (id, referrer_id, referee_id, code, code_kind, source, status, created_at)
VALUES (sqlc.arg(id), sqlc.arg(referrer_id), sqlc.arg(referee_id), sqlc.arg(code), sqlc.arg(code_kind), sqlc.arg(source), 'attributed', sqlc.arg(attributed_at))
ON CONFLICT (referee_id) DO NOTHING
RETURNING id;
