package adapters

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func seedStockListing(t *testing.T) (*StockListings, multichain.StockID) {
	t.Helper()
	pool := testkit.DB(t)
	g := testkit.NewIDs(4975)
	asset, listing := g.NewV7(), g.NewV7()
	now := clock.Real{}.Now().UTC()
	_, err := pool.Exec(t.Context(), `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name,
		ui_multiplier_num, ui_multiplier_den, issuer_tradable, company_key, first_seen_at, updated_at)
		VALUES ($1, 'LSTx', 'Xs4kgXGPAy68o3PXqnMyBzmeNnmXwBvq8gqjqNRbFi8', 8, 'xstocks', 'equity', 'L', 1, 1, true, 'l', $2, $2)`,
		asset, now)
	if err != nil {
		t.Fatal(err)
	}
	for i, enabled := range []bool{true, false} {
		_, err = pool.Exec(t.Context(), `INSERT INTO asset_listings (id, asset_id, issuer, symbol, network, chain_id,
			token_address, decimals, shares_per_token, venue, enabled, verified_at, created_at)
			VALUES ($1, $2, 'ondo', 'LSTon', 'ethereum', 1, $3, 18, 1.003376073740221058, 'uniswap', $4, $5, $5)`,
			[]any{listing, g.NewV7()}[i], asset, []string{
				"0x1100000000000000000000000000000000000001",
				"0x1100000000000000000000000000000000000002",
			}[i], enabled, now)
		if err != nil {
			t.Fatal(err)
		}
	}
	var stock multichain.StockID
	if err := stock.UnmarshalText([]byte(asset.String())); err != nil {
		t.Fatal(err)
	}
	listings := NewStockListings(pool)
	return &listings, stock
}

func TestStockListingsReturnsOnlyEnabledListingsOfTheStock(t *testing.T) {
	t.Parallel()
	listings, stock := seedStockListing(t)
	got, err := listings.EnabledListings(t.Context(), stock)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Network != chain.NetworkEthereum || got[0].Decimals != 18 || got[0].Issuer != "ondo" ||
		got[0].SharesPerToken.RatString() != "501688036870110529/500000000000000000" || got[0].StockID != stock ||
		got[0].Venue != multichain.VenueUniswap {
		t.Fatalf("listings = %+v", got)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := listings.EnabledListings(canceled, stock); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("err = %v, want internal", err)
	}
}
