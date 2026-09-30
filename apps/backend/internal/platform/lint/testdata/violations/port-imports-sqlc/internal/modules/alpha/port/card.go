package port

import "github.com/monaco/monaco/apps/backend/internal/modules/alpha/sqlc"

func Handle(q sqlc.Queries) string {
	return q.Handle
}
