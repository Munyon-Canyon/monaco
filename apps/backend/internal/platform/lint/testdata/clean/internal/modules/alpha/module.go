package alpha

import "github.com/monaco/monaco/apps/backend/internal/modules/identity"

func New(q identity.Queries) int {
	return q.Count()
}
