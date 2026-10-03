package treasury_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestSeed_cabalWithConfirmedTradePostsTheSwapAndAConfirmedActivityRow(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal, err := ids.ParseCabalID("01890a5d-ac96-774b-bcce-b302099a8090")
	if err != nil {
		t.Fatal(err)
	}
	testkit.NewLedger(t, f.pool).WithFundedMember(f.user(t), cabal, money.MicrosFromUint64(100_000_000))
	m := treasury.New(module.Deps{Config: testkit.Config(), Pool: f.pool, Clock: f.clock})
	if seeded := testkit.Seed(t, f.pool, "cabal-with-confirmed-trade", m.Consumers()...); len(seeded) != 2 {
		t.Fatalf("seeded %d events, want trade.submitted and trade.confirmed", len(seeded))
	}
	headers := f.swapHeaders(t)
	if len(headers) != 1 || !headers[0].CreatedAt.Equal(time.Date(2026, 3, 1, 12, 0, 30, 0, time.UTC)) {
		t.Fatalf("swap headers = %+v, want one stamped at the confirmation", headers)
	}
	wantPositions(t, f,
		position{Asset: usdcMint, Units: "75000000", Cost: "75000000"},
		position{Asset: string(aapl), Units: "105000000", Cost: "25000000"})
	got := f.activity(t)
	if len(got) != 1 {
		t.Fatalf("activity = %+v, want one row", got)
	}
	wantActivity(t, got[0], "buy", "confirmed", str(string(aapl)), str("25000000"), str("105000000"),
		str(string(swapSig)))
}
