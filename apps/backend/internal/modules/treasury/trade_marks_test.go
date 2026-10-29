package treasury_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const marksMint = "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"

func seedMarkTrade(
	t *testing.T,
	pool *pgxpool.Pool,
	cabal ids.CabalID,
	id uuid.UUID,
	kind, status string,
	usdc, units int64,
	at time.Time,
) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `INSERT INTO cabal_activity (id, cabal_id, kind, status, asset,
		usdc_micros, units, occurred_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)`,
		id, cabal.UUID(), kind, status, marksMint, usdc, units, at); err != nil {
		t.Fatal(err)
	}
}

type fixedNames struct{}

func (fixedNames) AssetNames(context.Context) (map[domain.Asset]app.AssetName, error) {
	return map[domain.Asset]app.AssetName{marksMint: {Symbol: "AAPLx", Decimals: 8}}, nil
}

func TestTradeMarks_replaysEachPairInTimeOrderFromOneStatement(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	seedAAPLx(t, pool)
	g := testkit.NewIDs(61)
	one, other := ids.CabalIDFrom(g.NewV7()), ids.CabalIDFrom(g.NewV7())
	base := clock.Real{}.Now().UTC().Truncate(time.Second).Add(-24 * time.Hour)
	buy1, sell1, buy2, otherBuy := g.NewV7(), g.NewV7(), g.NewV7(), g.NewV7()
	seedMarkTrade(t, pool, one, buy2, "buy", "confirmed", 500_000_000, 100_000_000, base.Add(3*time.Hour))
	seedMarkTrade(t, pool, one, buy1, "buy", "confirmed", 300_000_000, 100_000_000, base)
	seedMarkTrade(t, pool, one, sell1, "sell", "confirmed", 400_000_000, 100_000_000, base.Add(time.Hour))
	seedMarkTrade(t, pool, one, g.NewV7(), "buy", "pending", 1, 1, base.Add(4*time.Hour))
	seedMarkTrade(t, pool, other, otherBuy, "buy", "confirmed", 200_000_000, 100_000_000, base.Add(2*time.Hour))
	marks := treasury.New(module.Deps{Pool: pool}).TradeMarks()
	pairs := []port.CabalAsset{
		{CabalID: one, Symbol: "AAPLx"},
		{CabalID: other, Symbol: "AAPLx"},
		{CabalID: one, Symbol: "AAPLx"},
		{CabalID: one, Symbol: "NOPE"},
	}
	got, err := marks.TradeMarks(t.Context(), pairs)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("pairs = %d, want 2 (duplicate and unknown symbol dropped)", len(got))
	}
	list := got[pairs[0]]
	if len(list) != 3 || list[0].SwapID != buy1 || list[1].SwapID != sell1 || list[2].SwapID != buy2 {
		t.Fatalf("marks = %+v, want buy, sell, buy in time order without the pending row", list)
	}
	if list[1].CostOfSoldMicros.Uint64() != 300_000_000 || !list[1].CycleStart.Equal(base) ||
		!list[2].CycleStart.Equal(base.Add(3*time.Hour)) || list[0].PriceMicros.Uint64() != 300_000_000 {
		t.Fatalf("marks = %+v, want sale cost 300000000, its cycle at the first buy, the repurchase a new cycle", list)
	}
	assertOneBuy(t, got[pairs[1]], otherBuy)
}

func assertOneBuy(t *testing.T, marks []port.TradeMark, id uuid.UUID) {
	t.Helper()
	if len(marks) != 1 || marks[0].SwapID != id {
		t.Fatalf("other cabal marks = %+v, want its one buy", marks)
	}
}

func TestTradeMarks_answersAnEmptyAskWithoutTouchingTheDatabase(t *testing.T) {
	t.Parallel()
	marks := app.NewTradeMarks(nil, failingNames{})
	if got, err := marks.TradeMarks(t.Context(), nil); err != nil || len(got) != 0 {
		t.Fatalf("empty ask = %v, %v, want an empty map", got, err)
	}
}

func TestTradeMarks_passesFailuresOn(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	seedAAPLx(t, pool)
	names := treasury.CatalogNames{Catalog: market.New(module.Deps{Pool: pool}).Catalog()}
	cabal := ids.CabalIDFrom(testkit.NewIDs(62).NewV7())
	pair := []port.CabalAsset{{CabalID: cabal, Symbol: "AAPLx"}}
	if _, err := app.NewTradeMarks(pool, failingNames{}).
		TradeMarks(t.Context(), pair); errs.CodeOf(
		err,
	) != errs.CodeDBUnavailable {
		t.Fatalf("names down: err = %v, want db_unavailable", err)
	}
	if got, err := app.NewTradeMarks(pool, names).
		TradeMarks(t.Context(), []port.CabalAsset{{CabalID: cabal, Symbol: "NOPE"}}); err != nil ||
		len(got) != 0 {
		t.Fatalf("unknown symbol = %v, %v, want an empty map", got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := app.NewTradeMarks(pool, fixedNames{}).
		TradeMarks(ctx, pair); errs.CodeOf(
		err,
	) != errs.CodeDBUnavailable {
		t.Fatalf("canceled read: err = %v, want db_unavailable", err)
	}
	seedMarkTrade(t, pool, cabal, testkit.NewIDs(63).NewV7(), "sell", "confirmed", 1, 1, clock.Real{}.Now().UTC())
	if _, err := app.NewTradeMarks(pool, names).TradeMarks(t.Context(), pair); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("sale without a position: err = %v, want internal", err)
	}
	seedMarkTrade(
		t,
		pool,
		cabal,
		testkit.NewIDs(64).NewV7(),
		"buy",
		"confirmed",
		1,
		0,
		clock.Real{}.Now().UTC().Add(-time.Hour),
	)
	if _, err := app.NewTradeMarks(pool, names).TradeMarks(t.Context(), pair); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("zero-unit trade: err = %v, want internal", err)
	}
}
