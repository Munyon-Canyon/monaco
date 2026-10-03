package feedtest

import (
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func Item(tb testing.TB, db sqlc.DBTX, c clock.Clock, g ids.Generator, edits ...func(*feed.Item)) uuid.UUID {
	tb.Helper()
	item := feed.Item{
		Kind:  feed.KindTrade,
		RefID: g.NewV7(),
		Payload: feed.Payload{
			CabalName: "Alpha Cabal", Symbol: "AAPLx", AssetName: "Apple", Action: feed.ActionBuy,
			USDCMicros: money.MicrosFromUint64(500_000_000),
		},
	}
	for _, edit := range edits {
		edit(&item)
	}
	id, err := adapters.NewFeedStore(db).UpsertItem(tb.Context(), g.NewV7(), item, c.Now())
	if err != nil {
		tb.Fatalf("feedtest.Item: %v", err)
	}
	return id
}
