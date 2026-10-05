-- name: ProposedMints :many
SELECT DISTINCT mint FROM proposals WHERE status IN ('open', 'passed') ORDER BY mint;
