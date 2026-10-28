package market_test

import (
	"context"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestStarters_servedOnTheAssetsRouteWithTheCabalCount(t *testing.T) {
	t.Parallel()
	counts := func(context.Context) ([]app.HolderCount, error) {
		return []app.HolderCount{
			{Mint: marketfake.TSLAx().Mint.Address(), Cabals: 41},
			{Mint: marketfake.AAPLx().Mint.Address(), Cabals: 2},
		}, nil
	}
	s := newMarketAPIMounting(t, marketWhen(), func(d module.Deps) func(apibase.Mount) {
		m := market.New(d)
		market.WithHolders(m, counts)
		return m.Mount
	})
	s.seedFixtures(t)
	page := pageOf(t, s.get(t, "/v1/assets?filter=in_cabals&limit=2"))
	if len(page.Assets) != 2 || page.NextCursor != nil {
		t.Fatalf("in_cabals over http = %+v", page)
	}
	if first := page.Assets[0]; first.Symbol != "TSLAx" || first.CabalCount == nil || *first.CabalCount != 41 {
		t.Fatalf("first row = %+v, want TSLAx with 41 cabals", first)
	}
	if second := page.Assets[1]; second.CabalCount != nil {
		t.Fatalf("second row = %+v, want a popular fill without a count", second)
	}
	for _, filter := range []string{"all", "popular", "pre_ipo"} {
		for _, row := range pageOf(t, s.get(t, "/v1/assets?filter="+filter)).Assets {
			if row.CabalCount != nil {
				t.Fatalf("filter=%s row %s carries a cabal count", filter, row.Symbol)
			}
		}
	}
	if rec := s.get(t, "/v1/assets?filter=in_cabals&cursor=c0FBUEx4"); rec.Code != 400 {
		t.Fatalf("in_cabals with a cursor = %d, want 400", rec.Code)
	}
}

type holderModule struct {
	module.Module
	counts []treasuryport.HolderCount
	err    error
}

func (holderModule) Name() string { return "holderstub" }

func (h holderModule) CabalHolderCounts(context.Context) ([]treasuryport.HolderCount, error) {
	return h.counts, h.err
}

func TestModule_wiresTheTreasuryHolderCountsIntoTheAssetList(t *testing.T) {
	t.Parallel()
	d := module.Deps{Pool: testkit.DB(t), Config: moduleConfig(), Clock: clock.Real{}}
	m := market.New(d)
	if market.Holders(m) != nil {
		t.Fatal("holders wired before the set exists")
	}
	module.NewSet(m, holderModule{counts: []treasuryport.HolderCount{{Mint: "MintHeld", Cabals: 4}}})
	got, err := market.Holders(m)(t.Context())
	if want := []app.HolderCount{{Mint: "MintHeld", Cabals: 4}}; err != nil || !slices.Equal(got, want) {
		t.Fatalf("holders = %+v, %v, want %+v", got, err, want)
	}
}

func TestModule_passesAHolderCountFailureThrough(t *testing.T) {
	t.Parallel()
	d := module.Deps{Pool: testkit.DB(t), Config: moduleConfig(), Clock: clock.Real{}}
	m := market.New(d)
	boom := errs.New(errs.CodeInternal, "market.test")
	module.NewSet(m, holderModule{err: boom})
	if _, err := market.Holders(m)(t.Context()); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("holders error = %v, want internal", err)
	}
}
