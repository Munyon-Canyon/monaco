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
