package domain

import "github.com/monaco/monaco/apps/backend/internal/modules/identity"

func Count(q identity.Queries) int {
	return q.Count()
}
