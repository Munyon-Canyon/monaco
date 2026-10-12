package app

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const listingTestMint = "Xs4kgXGPAy68o3PXqnMyBzmeNnmXwBvq8gqjqNRbFi8"

func seedListedAsset(t *testing.T, pool sqlc.DBTX) domain.AssetID {
	t.Helper()
	g := testkit.NewIDs(4969)
	id := domain.NewAssetID(g)
	now := clock.Real{}.Now().UTC()
	const asset = `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name, ui_multiplier_num,
		ui_multiplier_den, issuer_tradable, company_key, first_seen_at, updated_at)
		VALUES ($1, 'LSTx', $2, 8, 'xstocks', 'equity', 'Listing Test', 1, 1, true, 'listing test', $3, $3)`
	if _, err := pool.Exec(t.Context(), asset, id.UUID(), listingTestMint, now); err != nil {
		t.Fatal(err)
	}
	const listing = `INSERT INTO asset_listings (id, asset_id, issuer, symbol, network, chain_id, token_address,
		decimals, shares_per_token, venue, enabled, verified_at, created_at)
		VALUES ($1, $2, 'ondo', 'LSTon', 'ethereum', 1, $3, $4, $5, 'uniswap', false, $6, $7)`
	rows := []struct {
		token    string
		decimals any
		shares   string
		verified any
	}{
		{"0x" + "11" + "00000000000000000000000000000000000001", int16(18), "1.003376073740221058", now},
		{"0x" + "11" + "00000000000000000000000000000000000002", int16(18), "1", nil},
		{"0x" + "11" + "00000000000000000000000000000000000003", int16(18), "1.0000000000000000000001", now},
	}
	for _, r := range rows {
		_, err := pool.Exec(t.Context(), listing, g.NewV7(), id.UUID(), r.token, r.decimals, r.shares, r.verified, now)
		if err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func TestAssetByMintResolvesAnEVMListingToItsAssetWithTheListingDecimals(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	id := seedListedAsset(t, pool)
	c := NewCatalog(pool)
	mint, err := domain.ParseMint("ethereum:0x1100000000000000000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.AssetByMint(t.Context(), mint)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != id || got.Decimals != 18 || got.Mint != mint || got.Symbol != "LSTx" {
		t.Fatalf("asset = %+v", got)
	}
	if got.UIMultiplier != (domain.Multiplier{Num: 501688036870110529, Den: 500000000000000000}) {
		t.Fatalf("multiplier = %+v, want the listing's shares per token", got.UIMultiplier)
	}
	solana, err := domain.ParseMint(listingTestMint)
	if err != nil {
		t.Fatal(err)
	}
	if plain, err := c.AssetByMint(t.Context(), solana); err != nil || plain.ID != id || plain.Decimals != 8 {
		t.Fatalf("Solana mint = %+v, %v", plain, err)
	}
}

func TestAssetByMintRefusesUnverifiedUnknownAndUnrepresentableListings(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	seedListedAsset(t, pool)
	c := NewCatalog(pool)
	for raw, want := range map[string]errs.Code{
		"ethereum:0x1100000000000000000000000000000000000002": errs.CodeAssetNotFound,
		"base:0x1100000000000000000000000000000000000001":     errs.CodeAssetNotFound,
		"ethereum:0x1100000000000000000000000000000000000003": errs.CodeDecodeFailed,
	} {
		mint, err := domain.ParseMint(raw)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.AssetByMint(t.Context(), mint); errs.CodeOf(err) != want {
			t.Errorf("AssetByMint(%s) = %v, want %s", raw, err, want)
		}
	}
}

func listingIDOf(t *testing.T, pool sqlc.DBTX, token string) multichain.ListingID {
	t.Helper()
	var id uuid.UUID
	const q = `SELECT id FROM asset_listings WHERE token_address = $1`
	if err := pool.QueryRow(t.Context(), q, token).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return multichain.ListingIDFrom(id)
}

func TestListingReturnsAVerifiedListingWithItsStock(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	asset := seedListedAsset(t, pool)
	c := NewCatalog(pool)
	id := listingIDOf(t, pool, "0x1100000000000000000000000000000000000001")
	got, err := c.Listing(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != id || got.StockID.UUID() != asset.UUID() || got.Network != chain.NetworkEthereum ||
		got.TokenAddress != "0x1100000000000000000000000000000000000001" || got.Decimals != 18 ||
		got.SharesPerToken.RatString() != "501688036870110529/500000000000000000" ||
		got.Venue != multichain.VenueUniswap || got.Issuer != "ondo" || got.Symbol != "LSTon" {
		t.Fatalf("listing = %+v", got)
	}
}

func TestListingLeavesTheStockZeroForAnUnlinkedListing(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	var id uuid.UUID
	err := pool.QueryRow(t.Context(), `SELECT id FROM asset_listings
		WHERE asset_id IS NULL AND verified_at IS NOT NULL AND issuer = 'coinbase' ORDER BY id LIMIT 1`).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	got, err := NewCatalog(pool).Listing(t.Context(), multichain.ListingIDFrom(id))
	if err != nil || !got.StockID.IsZero() || got.Network != chain.NetworkBase {
		t.Fatalf("listing = %+v, %v, want a Base listing with no stock", got, err)
	}
}

func TestListingRefusesUnknownUnverifiedAndUnreadableListings(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	seedListedAsset(t, pool)
	c := NewCatalog(pool)
	v4Asset := uuid.UUID{0x1f, 0x0c, 0x6a, 0x4e, 0x7d, 0x0b, 0x4c, 0x3a, 0x9e, 0x2f, 0x5b, 0x8d, 0x1a, 0x6c, 0x3e, 0x70}
	now := clock.Real{}.Now().UTC()
	_, err := pool.Exec(t.Context(), `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name,
		ui_multiplier_num, ui_multiplier_den, issuer_tradable, company_key, first_seen_at, updated_at)
		VALUES ($1, 'V4x', 'Xs8S1uUs1zvS2p7iwtsG3b6fkhpvmwz4GYU3gWAmWHZ', 8, 'xstocks', 'equity', 'V4', 1, 1, true, 'v4', $2, $2)`,
		v4Asset, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(t.Context(), `INSERT INTO asset_listings (id, asset_id, issuer, symbol, network, chain_id,
		token_address, decimals, shares_per_token, venue, enabled, verified_at, created_at)
		VALUES ($1, $2, 'ondo', 'V4on', 'ethereum', 1, '0x1100000000000000000000000000000000000009', 18, 1, 'uniswap',
		false, $3, $3)`, testkit.NewIDs(4970).NewV7(), v4Asset, now)
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		id   multichain.ListingID
		want errs.Code
	}{
		"unknown":    {multichain.ListingIDFrom(testkit.NewIDs(4971).NewV7()), errs.CodeAssetNotFound},
		"unverified": {listingIDOf(t, pool, "0x1100000000000000000000000000000000000002"), errs.CodeAssetUntradable},
		"v4 stock":   {listingIDOf(t, pool, "0x1100000000000000000000000000000000000009"), errs.CodeDecodeFailed},
	} {
		if _, err := c.Listing(t.Context(), tc.id); errs.CodeOf(err) != tc.want {
			t.Errorf("%s: err = %v, want %s", name, err, tc.want)
		}
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	id := listingIDOf(t, pool, "0x1100000000000000000000000000000000000001")
	if _, err := c.Listing(canceled, id); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("canceled: err = %v, want internal", err)
	}
}

func TestListingByAssetResolvesASolanaMint(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	got, err := NewCatalog(pool).ListingByAsset(t.Context(), "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
	if err != nil || got.Network != chain.NetworkSolana || got.Symbol != "AAPLx" || got.Decimals != 8 ||
		!got.Enabled || got.Venue != multichain.VenueJupiter {
		t.Fatalf("Solana mint = %+v, %v", got, err)
	}
}

func TestListingByAssetKeepsADisabledEVMListingResolvable(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	seedListedAsset(t, pool)
	got, err := NewCatalog(pool).ListingByAsset(t.Context(), "ethereum:0x1100000000000000000000000000000000000001")
	if err != nil || got.Network != chain.NetworkEthereum || got.Decimals != 18 || got.Enabled ||
		got.TokenAddress != "0x1100000000000000000000000000000000000001" ||
		got.SharesPerToken.RatString() != "501688036870110529/500000000000000000" {
		t.Fatalf("disabled EVM listing = %+v, %v, want it resolved with Enabled false", got, err)
	}
}

func TestListingByAssetRefusesUnverifiedUnknownAndMalformedAssets(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	seedListedAsset(t, pool)
	c := NewCatalog(pool)
	for asset, want := range map[string]errs.Code{
		"ethereum:0x1100000000000000000000000000000000000002": errs.CodeAssetUntradable,
		"base:0x1100000000000000000000000000000000000001":     errs.CodeAssetNotFound,
		"solana:0x12": errs.CodeInvalidAddress,
	} {
		if _, err := c.ListingByAsset(t.Context(), asset); errs.CodeOf(err) != want {
			t.Errorf("ListingByAsset(%s) = %v, want %s", asset, err, want)
		}
	}
}

func TestListingOn_findsAnAssetsListingOnANetworkEvenWhenDisabled(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	id := seedListedAsset(t, pool)
	var stock multichain.StockID
	if err := stock.UnmarshalText([]byte(id.String())); err != nil {
		t.Fatal(err)
	}
	var want uuid.UUID
	if err := pool.QueryRow(t.Context(), `SELECT id FROM asset_listings WHERE token_address = $1`,
		"0x1100000000000000000000000000000000000001").Scan(&want); err != nil {
		t.Fatal(err)
	}
	c := NewCatalog(pool)
	listing, venue, err := c.ListingOn(t.Context(), stock, chain.NetworkEthereum)
	if err != nil || listing != multichain.ListingIDFrom(want) || venue != multichain.VenueUniswap {
		t.Fatalf("ListingOn(ethereum) = %s, %s, %v, want the first verified listing %s on uniswap",
			listing, venue, err, want)
	}
	if _, _, err := c.ListingOn(t.Context(), stock, chain.NetworkBase); errs.CodeOf(err) != errs.CodeAssetNotFound {
		t.Fatalf("ListingOn(base) err = %v, want asset_not_found", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := c.ListingOn(ctx, stock, chain.NetworkEthereum); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("ListingOn on a cancelled context err = %v, want internal", err)
	}
}
