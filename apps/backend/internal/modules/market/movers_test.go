package market_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const (
	heldMint  = "11111111111111111111111111111111"
	coldMint  = "So11111111111111111111111111111111111111112"
	tieMint   = "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"
	freshMint = "ComputeBudget111111111111111111111111111111"
	spaceMint = "SysvarC1ock11111111111111111111111111111111"
)

func (s marketAPI) move(t *testing.T, mint string, nowMicros int64) {
	t.Helper()
	s.price(t, mint, -18*time.Hour-time.Minute, 100_000_000)
	s.price(t, mint, -time.Minute, nowMicros)
}

func (s marketAPI) seedMovers(t *testing.T) {
	t.Helper()
	s.seedFixtures(t)
	s.move(t, marketfake.AAPLx().Mint.String(), 102_000_000)
	s.move(t, marketfake.TSLAx().Mint.String(), 95_000_000)
	s.insertClone(t, 4, "MSFTx", heldMint, "Microsoft xStock", domain.KindEquity, 0)
	s.move(t, heldMint, 108_000_000)
	s.insertClone(t, 5, "COLDx", coldMint, "Cold xStock", domain.KindEquity, 0)
	s.move(t, coldMint, 120_000_000)
	s.insertClone(t, 6, "ZZZx", tieMint, "Tie xStock", domain.KindEquity, 9)
	s.move(t, tieMint, 105_000_000)
	s.insertClone(t, 7, "NEWx", freshMint, "Fresh xStock", domain.KindEquity, 10)
	s.price(t, freshMint, -time.Minute, 50_000_000)
	s.insertClone(t, 8, "SPACEx", spaceMint, "SpaceX", domain.KindPreIPO, 11)
	s.move(t, spaceMint, 300_000_000)
}

func (s marketAPI) movers(t *testing.T, req app.ListRequest) app.Page {
	t.Helper()
	held := func(context.Context) ([]chain.SolanaAddress, error) {
		return []chain.SolanaAddress{chain.SolanaAddress(heldMint)}, nil
	}
	req.Filter = app.FilterMovers
	page, err := app.NewListAssets(s.pool, s.clock, held).Handle(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func symbolsOf(page app.Page) []string {
	out := make([]string, len(page.Items))
	for i, item := range page.Items {
		out[i] = item.Asset.Symbol
	}
	return out
}

func TestMovers_ranksPricedTradableStocksByTheSizeOfTheMove(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedMovers(t)
	page := s.movers(t, app.ListRequest{Limit: 5})
	want := []string{"MSFTx", "TSLAx", "ZZZx", "AAPLx"}
	if got := symbolsOf(page); !slices.Equal(got, want) || page.NextCursor != "" {
		t.Fatalf("movers = %v next %q, want %v", got, page.NextCursor, want)
	}
	if first := page.Items[0]; first.Change == nil || *first.Change != 800 || len(first.Sparkline) == 0 {
		t.Fatalf("first mover = %+v, want +800 bps with a sparkline", first)
	}
}

func TestMovers_pagesEveryCandidateOnceInOrder(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedMovers(t)
	var got []string
	cursor := ""
	for range 10 {
		page := s.movers(t, app.ListRequest{Limit: 1, Cursor: cursor})
		got = append(got, symbolsOf(page)...)
		if cursor = page.NextCursor; cursor == "" {
			break
		}
	}
	if want := []string{"MSFTx", "TSLAx", "ZZZx", "AAPLx"}; !slices.Equal(got, want) {
		t.Fatalf("paged movers = %v, want %v", got, want)
	}
}

func TestMovers_searchNarrowsAndKeepsTheOrder(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedMovers(t)
	if got := symbolsOf(s.movers(t, app.ListRequest{Q: "micro"})); !slices.Equal(got, []string{"MSFTx"}) {
		t.Fatalf("q=micro = %v", got)
	}
	if got := symbolsOf(s.movers(t, app.ListRequest{Q: "x"})); !slices.Equal(got, []string{"MSFTx", "ZZZx"}) {
		t.Fatalf("q=x = %v", got)
	}
}

func TestMovers_servedOnTheAssetsRoute(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedMovers(t)
	page := pageOf(t, s.get(t, "/v1/assets?filter=movers&limit=2"))
	first, second := page.Assets[0].Symbol, page.Assets[1].Symbol
	if len(page.Assets) != 2 || first != "TSLAx" || second != "ZZZx" || page.NextCursor == nil {
		t.Fatalf("movers over http = %+v", page)
	}
}
