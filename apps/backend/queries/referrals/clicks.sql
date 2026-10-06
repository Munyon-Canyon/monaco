-- name: CountReferralClick :exec
INSERT INTO referral_clicks (code, day, clicks)
VALUES (sqlc.arg(code), to_date(sqlc.arg(day)::text, 'YYYY-MM-DD'), 1)
ON CONFLICT (code, day) DO UPDATE SET clicks = referral_clicks.clicks + 1;
