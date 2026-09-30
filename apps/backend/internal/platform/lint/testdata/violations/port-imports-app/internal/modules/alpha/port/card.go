package port

import "github.com/monaco/monaco/apps/backend/internal/modules/alpha/app"

func CardID(c app.Card) string {
	return c.ID
}
