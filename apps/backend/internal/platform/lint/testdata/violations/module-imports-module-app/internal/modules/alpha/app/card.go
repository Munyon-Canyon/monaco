package app

import identityapp "github.com/monaco/monaco/apps/backend/internal/modules/identity/app"

func CardID(c identityapp.Card) string {
	return c.ID
}
