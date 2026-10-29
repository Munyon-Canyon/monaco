-- name: ProposedMints :many
SELECT DISTINCT mint::text AS mint FROM proposals WHERE status IN ('open', 'passed') AND mint IS NOT NULL ORDER BY mint;
