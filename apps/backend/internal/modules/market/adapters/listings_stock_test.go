package adapters

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func seedStockListing(t *testing.T) (*StockListings, multichain.StockID) {
	t.Helper()
	listings, stock, _ := seedStockListingIn(t)
	return listings, stock
}

func seedStockListingIn(t *testing.T) (*StockListings, multichain.StockID, *pgxpool.Pool) {
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
	return &listings, stock, pool
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
		got[0].Venue != multichain.VenueUniswap || !got[0].Enabled {
		t.Fatalf("listings = %+v", got)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := listings.EnabledListings(canceled, stock); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("err = %v, want internal", err)
	}
}

func TestStockListingsKeepsSolanaToTheAssetsOwnMint(t *testing.T) {
	t.Parallel()
	listings, stock, pool := seedStockListingIn(t)
	g := testkit.NewIDs(7515)
	now := clock.Real{}.Now().UTC()
	own, ondo := g.NewV7(), g.NewV7()
	rows := []struct {
		id     any
		issuer string
		token  string
	}{
		{own, "xstocks", "Xs4kgXGPAy68o3PXqnMyBzmeNnmXwBvq8gqjqNRbFi8"},
		{ondo, "ondo", "OndoSo1anaMint11111111111111111111111111111"},
	}
	for _, row := range rows {
		if _, err := pool.Exec(t.Context(), `INSERT INTO asset_listings (id, asset_id, issuer, symbol, network,
			token_address, decimals, shares_per_token, venue, enabled, verified_at, created_at)
			VALUES ($1, $2, $3, 'LST', 'solana', $4, 8, 1, 'jupiter', true, $5, $5)`,
			row.id, stock.UUID(), row.issuer, row.token, now); err != nil {
			t.Fatal(err)
		}
	}
	got, err := listings.EnabledListings(t.Context(), stock)
	if err != nil {
		t.Fatal(err)
	}
	solana := make([]string, 0, len(got))
	for _, l := range got {
		if l.Network == chain.NetworkSolana {
			solana = append(solana, l.TokenAddress)
		}
	}
	if len(solana) != 1 || solana[0] != "Xs4kgXGPAy68o3PXqnMyBzmeNnmXwBvq8gqjqNRbFi8" {
		t.Fatalf("solana listings = %v, want only the asset's own mint", solana)
	}
	if len(got) != 2 {
		t.Fatalf("listings = %d, want the ethereum listing and the own solana mint", len(got))
	}
}
