package adapters_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestLatest_CabalReturnsFailsWhenTheDatabaseCallFails(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	cabal := ids.CabalIDFrom(testkit.NewIDs(1).NewV7())
	latest := adapters.Latest{DB: testkit.DB(t)}
	if _, err := latest.CabalReturns(ctx, port.Range1M, []ids.CabalID{cabal}); err == nil {
		t.Fatal("CabalReturns() error = nil")
	}
}
