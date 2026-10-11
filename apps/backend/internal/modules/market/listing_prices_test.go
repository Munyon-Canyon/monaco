package market_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func seedListing(t *testing.T, pool *pgxpool.Pool, asset market.Asset, network chain.Network) string {
	t.Helper()
	raw := make([]byte, 20)
	_, _ = rand.Read(raw)
	token := "0x" + hex.EncodeToString(raw)
	if _, err := pool.Exec(
		t.Context(),
		`INSERT INTO asset_listings (id, asset_id, issuer, symbol, network, chain_id,
		token_address, decimals, shares_per_token, venue, enabled, created_at)
		VALUES ($1, $2, 'ondo', $3, $4, $5, $6, 18, 1.25, 'uniswap', false, now())`,
		testkit.NewIDs(29).
			NewV7(),
		asset.ID.UUID(),
		asset.Symbol+"on",
		string(network),
		network.EVMChainID(),
		token,
	); err != nil {
		t.Fatal(err)
	}
	return token
}

type listingRig struct {
	pool   *pgxpool.Pool
	clock  *testkit.Clock
	bucket time.Time
	token  string
	key    string
}

func newListingRig(t *testing.T) listingRig {
	t.Helper()
	pool := testkit.DB(t)
	bucket := clock.Real{}.Now().UTC().Truncate(domain.SampleBucket)
	aapl := marketfake.AAPLx()
	seedAssets(t, pool, bucket, aapl)
	token := seedListing(t, pool, aapl, chain.NetworkBase)
	return listingRig{
		pool: pool, clock: testkit.NewClock(bucket.Add(37 * time.Second)), bucket: bucket, token: token,
		key: "base:" + token,
	}
}

func (r listingRig) price(t *testing.T, at time.Time, micros int64) {
	t.Helper()
	if _, err := r.pool.Exec(t.Context(), `INSERT INTO price_points (mint, ts, price_micros, source)
		VALUES ($1, $2, $3, 'uniswap')`, r.key, at, micros); err != nil {
		t.Fatal(err)
	}
}

func (r listingRig) read(t *testing.T) (map[app.ListingKey]app.ListingPrice, error) {
	t.Helper()
	keys := []app.ListingKey{app.ListingKey(r.key), "base:0xunknown"}
	return app.NewPriceBook(r.pool, r.clock).ListingPrices(t.Context(), keys, r.clock.Now())
}

func TestListingPrices_readsAHeldListingWithItsCatalogStockAndAcceptedPrice(t *testing.T) {
	t.Parallel()
	r := newListingRig(t)
	wantUnpricedAAPLListing(t, r)
	r.price(t, r.bucket.Add(-4*time.Minute), 250_000_000)
	r.price(t, r.bucket.Add(-2*time.Minute), 251_000_000)
	r.price(t, r.bucket, 252_000_000)
	r.price(t, r.bucket.Add(time.Hour), 999_000_000)
	got, err := r.read(t)
	if l := got[app.ListingKey(r.key)]; err != nil || l.Price == nil || l.Price.Micros.Uint64() != 252_000_000 ||
		!l.Price.ObservedAt.Equal(r.bucket) {
		t.Fatalf("ListingPrices = %+v, %v, want the newest sample at or before now", got, err)
	}
}

func wantUnpricedAAPLListing(t *testing.T, r listingRig) {
	t.Helper()
	got, err := r.read(t)
	if err != nil {
		t.Fatal(err)
	}
	l, ok := got[app.ListingKey(r.key)]
	if len(got) != 1 || !ok || l.Price != nil || l.SharesPerToken.String() != "5/4" || l.AssetID == nil ||
		*l.AssetID != marketfake.AAPLx().ID || l.Asset.Mint != marketfake.AAPLx().Mint.String() {
		t.Fatalf("ListingPrices before any sample = %+v, want the listing linked to AAPLx with no price", got)
	}
}

func TestListingPrices_aStoredPriceThatDoesNotDecodeFailsTheRead(t *testing.T) {
	t.Parallel()
	r := newListingRig(t)
	r.price(t, r.bucket, -1)
	if _, err := r.read(t); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("ListingPrices over a negative price = %v, want decode_failed", err)
	}
}

func TestListingPrices_aListingLinkedToAnUnreadableAssetIDFailsTheRead(t *testing.T) {
	t.Parallel()
	r := newListingRig(t)
	const v4 = "6ba7b810-9dad-41d1-80b4-00c04fd430c8"
	if _, err := r.pool.Exec(t.Context(), `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name,
		issuer_tradable, company_key, first_seen_at, updated_at)
		VALUES ($1, 'V4x', 'V4xMint1111111111111111111111111111111111111', 8, 'xstocks', 'equity', 'V4', true, 'V4', $2, $2)`,
		v4, r.bucket); err != nil {
		t.Fatal(err)
	}
	if _, err := r.pool.Exec(t.Context(), `UPDATE asset_listings SET asset_id = $1 WHERE token_address = $2`,
		v4, r.token); err != nil {
		t.Fatal(err)
	}
	if _, err := r.read(t); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("ListingPrices over a v4 asset id = %v, want decode_failed", err)
	}
}

func TestListingPrices_aFailedReadReturnsItsError(t *testing.T) {
	t.Parallel()
	r := newListingRig(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := app.NewPriceBook(r.pool, r.clock).ListingPrices(ctx, nil, r.clock.Now()); err == nil {
		t.Fatal("ListingPrices on a cancelled context = nil, want an error")
	}
}

var _ market.ListingPrices = (*app.PriceBook)(nil)
