package ranking

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

type unlistedMarket struct{ app.Market }

func (unlistedMarket) ListAll(context.Context) ([]market.Asset, error) {
	return nil, errs.New(errs.CodeDBUnavailable, "fixture.ListAll")
}

func TestPinnedMarket_failsWhenTheAssetsCannotBeListed(t *testing.T) {
	t.Parallel()
	pinned := pinnedMarket{Market: unlistedMarket{}, at: valuationTime()}
	if _, err := pinned.LatestPrices(t.Context()); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("LatestPrices err = %v, want db_unavailable", err)
	}
}

func TestProjection_isEmptyWithoutASourceRankingModule(t *testing.T) {
	t.Parallel()
	if got := New(module.Deps{}).Projection(module.Set{}); got != nil {
		t.Fatalf("Projection = %v, want none", got)
	}
}
