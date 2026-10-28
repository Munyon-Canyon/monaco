-- name: GetProposalHead :one
SELECT kind, symbol, coalesce(thesis, '')::text AS thesis FROM proposals WHERE id = @id;
