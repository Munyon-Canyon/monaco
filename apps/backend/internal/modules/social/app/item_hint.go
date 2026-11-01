package app

import (
	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func itemChangedKey(cabal ids.CabalID) string {
	if cabal.UUID() == uuid.Nil {
		return FeedHint
	}
	return events.CabalFeedItemChangedHint(cabal)
}
