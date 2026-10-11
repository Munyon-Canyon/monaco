package treasury_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const activityEVMAsset = "base:0xb200000000000000000000000000000000000c03"

func seedRoutedSwap(t *testing.T, pool *pgxpool.Pool, cabal ids.CabalID, swap uuid.UUID, at time.Time) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `INSERT INTO swaps (id, source_kind, source_id, cabal_id, treasury_address,
		action, symbol, in_mint, out_mint, out_decimals, in_amount, slippage_bps, source_batch_size, status,
		created_at, updated_at)
		VALUES ($1, 'proposal', $2, $3, 'treasury', 'sell', 'AAPLx', $4, 'usdc', 6, 1, 100, 1, 'confirmed', $5, $5)`,
		swap, testkit.NewIDs(71).NewV7(), cabal.UUID(), activityEVMAsset, at); err != nil {
		t.Fatal(err)
	}
	for i, step := range []string{"swap", "bridge_back"} {
		if _, err := pool.Exec(
			t.Context(),
			`INSERT INTO trade_steps (id, trade_id, step, network, amount_micros, status,
			created_at, updated_at) VALUES ($1, $2, $3, 'base', 25000000, $4, $5, $5)`,
			testkit.NewIDs(uint64(80+i)).NewV7(),
			swap,
			step,
			[]string{"succeeded", "submitted"}[i],
			at,
		); err != nil {
			t.Fatal(err)
		}
	}
}

func TestActivityReads_showsARoutedTradePendingUntilItsLastStepAndNamesItsStock(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	swap, at := testkit.NewIDs(70).NewV7(), clock.Real{}.Now().UTC().Truncate(time.Second)
	asset := activityEVMAsset
	seedActivity(t, pool, c.ID, seededActivity{id: swap, at: at, status: "confirmed", asset: &asset})
	seedRoutedSwap(t, pool, c.ID, swap, at)
	listings := app.ListingReader(
		func(_ context.Context, keys []string, _ time.Time) (map[string]app.ListingQuote, error) {
			out := map[string]app.ListingQuote{}
			for _, k := range keys {
				out[k] = app.ListingQuote{
					Symbol: "AAPLon", Network: chain.NetworkBase,
					Asset: &app.ListedAsset{Symbol: "AAPLx", DisplayName: "Apple"},
				}
			}
			return out, nil
		},
	)
	reads := app.NewActivityReads(pool, member(true), app.UnwiredReads{}, noNames{}).WithListings(listings)
	read := func() app.ActivityView {
		t.Helper()
		page, err := reads.List(t.Context(), app.ListActivity{CabalID: c.ID, Caller: c.Creator.ID})
		if err != nil || len(page.Items) != 1 {
			t.Fatalf("page = %+v, %v, want the one trade", page, err)
		}
		return page.Items[0]
	}
	if got := read(); got.Status != domain.ActivityPending || got.Asset == nil || got.Asset.Symbol != "AAPLx" ||
		got.Asset.Name != "Apple" {
		t.Fatalf("trade mid-bridge = %+v, want pending and named Apple (AAPLx)", got)
	}
	if _, err := pool.Exec(
		t.Context(),
		`UPDATE trade_steps SET status = 'succeeded' WHERE trade_id = $1`,
		swap,
	); err != nil {
		t.Fatal(err)
	}
	if got := read(); got.Status != domain.ActivityConfirmed {
		t.Fatalf("trade after its last step = %+v, want confirmed", got)
	}
}

func TestActivityReads_failsWhenTheListingNamesCannotBeRead(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	asset := activityEVMAsset
	seedActivity(t, pool, c.ID, seededActivity{
		id: testkit.NewIDs(72).NewV7(), at: clock.Real{}.Now().UTC(), status: "confirmed", asset: &asset,
	})
	down := errs.New(errs.CodeDBUnavailable, "test")
	reads := app.NewActivityReads(pool, member(true), app.UnwiredReads{}, noNames{}).WithListings(
		func(context.Context, []string, time.Time) (map[string]app.ListingQuote, error) { return nil, down })
	if _, err := reads.List(t.Context(), app.ListActivity{CabalID: c.ID, Caller: c.Creator.ID}); errs.CodeOf(err) !=
		errs.CodeDBUnavailable {
		t.Fatalf("List over a failed listing read = %v, want db_unavailable", err)
	}
	unwired := app.NewActivityReads(pool, member(true), app.UnwiredReads{}, noNames{})
	page, err := unwired.List(t.Context(), app.ListActivity{CabalID: c.ID, Caller: c.Creator.ID})
	if err != nil || len(page.Items) != 1 || page.Items[0].Asset != nil {
		t.Fatalf("List without a listing reader = %+v, %v, want the row unnamed", page, err)
	}
}

type noNames struct{}

func (noNames) AssetNames(context.Context) (map[domain.Asset]app.AssetName, error) {
	return map[domain.Asset]app.AssetName{}, nil
}
