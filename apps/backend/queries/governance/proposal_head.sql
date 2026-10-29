-- name: GetProposalHead :one
SELECT kind, coalesce(symbol, '')::text AS symbol, coalesce(thesis, '')::text AS thesis FROM proposals WHERE id = @id;
