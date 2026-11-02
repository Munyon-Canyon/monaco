package treasury_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	tenTokens    = "10000000000000000000"
	twelveTokens = "12000000000000000000"
	twentyFive   = "25000000000000000000"
)

func wideUnits(t *testing.T, raw string) money.Units {
	t.Helper()
	v, err := money.ParseUnits(raw)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func (f fixture) postSwap(t *testing.T, s domain.Swap) {
	t.Helper()
	err := f.do(func(ctx context.Context, tx db.Tx) error { return f.ledger.PostSwap(ctx, tx, s) })
	if err != nil {
		t.Fatalf("PostSwap %+v: %v", s, err)
	}
}

type wideStep struct {
	name   string
	swap   domain.Swap
	units  string
	cost   string
	usdcAt string
}

func (f fixture) wantWidePositions(t *testing.T, step wideStep) {
	t.Helper()
	got := f.cabalPositions(t)
	if len(got) != 2 || got[0].Asset != string(usdc) || got[0].Units != step.usdcAt ||
		got[1].Asset != string(aapl) || got[1].Units != step.units || got[1].Cost != step.cost {
		t.Fatalf("%s: positions = %+v, want %s tokens costing %s and %s USDC",
			step.name, got, step.units, step.cost, step.usdcAt)
	}
	if drift := f.drift(t); len(drift) != 0 {
		t.Fatalf("%s: drift = %q, want none", step.name, drift)
	}
}

func buyLeg(t *testing.T, f fixture, cabal ids.CabalID, usdcIn, tokens string) domain.Swap {
	t.Helper()
	return domain.Swap{
		ID: f.ids.NewV7(), CabalID: cabal, In: domain.Leg{Asset: usdc, Amount: wideUnits(t, usdcIn)},
		Out: domain.Leg{Asset: aapl, Amount: wideUnits(t, tokens)},
	}
}

func sellLeg(t *testing.T, f fixture, cabal ids.CabalID, tokens, usdcOut string) domain.Swap {
	t.Helper()
	return domain.Swap{
		ID: f.ids.NewV7(), CabalID: cabal, In: domain.Leg{Asset: aapl, Amount: wideUnits(t, tokens)},
		Out: domain.Leg{Asset: usdc, Amount: wideUnits(t, usdcOut)},
	}
}

func TestLedger_postsWideTokenBuysAndSellsAndKeepsPositionsExactAndDriftFree(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	u, c, err := f.fund(user, cabal, 400_000_000, 400_000_000, domain.TxnPending)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	steps := []wideStep{
		{"a 10 token buy", buyLeg(t, f, cabal, "50000000", tenTokens), tenTokens, "50000000", "350000000"},
		{
			"a 25 token buy",
			buyLeg(t, f, cabal, "125000000", twentyFive),
			"35000000000000000000",
			"175000000",
			"225000000",
		},
		{
			"a 12 token sell",
			sellLeg(t, f, cabal, twelveTokens, "70000000"),
			"23000000000000000000",
			"115000000",
			"295000000",
		},
		{"selling the rest", sellLeg(t, f, cabal, "23000000000000000000", "130000000"), "0", "0", "425000000"},
	}
	for _, step := range steps {
		f.postSwap(t, step.swap)
		f.wantWidePositions(t, step)
	}
}

func TestLedger_aWideSwapReplaysIdempotently(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	u, c, err := f.fund(user, cabal, 200_000_000, 200_000_000, domain.TxnPending)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	swap := domain.Swap{
		ID: f.ids.NewV7(), CabalID: cabal, In: domain.Leg{Asset: usdc, Amount: wideUnits(t, "125000000")},
		Out: domain.Leg{Asset: aapl, Amount: wideUnits(t, twentyFive)},
	}
	f.postSwap(t, swap)
	f.postSwap(t, swap)
	if got := f.cabalPositions(t); len(got) != 2 || got[1].Units != twentyFive {
		t.Fatalf("positions after posting the same swap twice = %+v, want 25 tokens once", got)
	}
}

func TestTradesConsumer_aWideConfirmationIsPostedOnceAndKeepsExactPositions(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	e := buy(cabal, f.ids.NewV7(), 60_000_000, 1, 0)
	e.OutAmount = wideUnits(t, twentyFive)
	for range 2 {
		if err := f.confirm(t, e); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(f.swapHeaders(t)); got != 1 {
		t.Fatalf("swap headers = %d, want 1", got)
	}
	wantPositions(t, f,
		position{Asset: usdcMint, Units: "40000000", Cost: "40000000"},
		position{Asset: string(aapl), Units: twentyFive, Cost: "60000000"})
	if drift := f.drift(t); len(drift) != 0 {
		t.Fatalf("drift = %q, want none", drift)
	}
}
