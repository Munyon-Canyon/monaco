package treasury_test

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	baseCash = "base:0x833589fcd6edb6e08f4c7c32d4f71b54bda02913"
	baseAAPL = "base:0xaa00000000000000000000000000000000000001"
)

type bridgeRun struct {
	f     fixture
	cabal ids.CabalID
	swap  uuid.UUID
	step  string
}

func newBridgeRun(t *testing.T, f fixture, cabal ids.CabalID, step string) bridgeRun {
	t.Helper()
	return bridgeRun{f: f, cabal: cabal, swap: f.ids.NewV7(), step: step}
}

func (r bridgeRun) event(status, action string, sent, fee uint64) events.TradeStepChanged {
	return events.TradeStepChanged{
		V: 1, SwapID: r.swap, CabalID: r.cabal.UUID(), Step: r.step, Network: chain.NetworkBase, Status: status,
		PrivyActionID: action, AmountMicros: money.MicrosFromUint64(sent), FeeMicros: money.MicrosFromUint64(fee),
	}
}

func (r bridgeRun) handle(t *testing.T, e events.TradeStepChanged) {
	t.Helper()
	if err := r.f.stepChanged(e); err != nil {
		t.Fatalf("%s %s: %v", e.Step, e.Status, err)
	}
}

func (r bridgeRun) submitted(t *testing.T, sent uint64) {
	t.Helper()
	r.handle(t, r.event("pending", "", sent, 0))
	r.handle(t, r.event("submitted", "privy-"+r.swap.String(), sent, 0))
}

func (r bridgeRun) succeeded(t *testing.T, sent, fee uint64) {
	t.Helper()
	r.handle(t, r.event("succeeded", "privy-"+r.swap.String(), sent, fee))
}

func (r bridgeRun) failed(t *testing.T, sent uint64) {
	t.Helper()
	r.handle(t, r.event("failed", "privy-"+r.swap.String(), sent, 0))
}

func (f fixture) stepChanged(e events.TradeStepChanged) error {
	h := adapters.TradeSteps{Ledger: f.ledger, Hints: &hints{}}
	return f.do(func(ctx context.Context, tx db.Tx) error { return h.Handle(ctx, tx, e, confirmedAt()) })
}

type inTransit struct {
	Swap    uuid.UUID
	Network string
	Micros  string
}

func (f fixture) inTransit(t *testing.T, cabal ids.CabalID) []inTransit {
	t.Helper()
	rows, err := f.pool.Query(t.Context(),
		`SELECT trade_id, network, amount_micros::text FROM treasury_in_transit WHERE cabal_id = $1 ORDER BY trade_id`,
		cabal.UUID())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []inTransit
	for rows.Next() {
		var r inTransit
		if err := rows.Scan(&r.Swap, &r.Network, &r.Micros); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func wantInTransit(t *testing.T, f fixture, cabal ids.CabalID, want ...inTransit) {
	t.Helper()
	if got := f.inTransit(t, cabal); !slices.Equal(got, want) {
		t.Fatalf("in transit = %+v, want %+v", got, want)
	}
}

func wantBridgePositions(t *testing.T, f fixture, want ...position) {
	t.Helper()
	if got := f.cabalPositions(t); !slices.Equal(got, want) {
		t.Fatalf("cabal positions = %+v, want %+v", got, want)
	}
}

func (f fixture) bridgedOut(t *testing.T, cabal ids.CabalID, sent, fee uint64) {
	t.Helper()
	out := newBridgeRun(t, f, cabal, "bridge_out")
	out.submitted(t, sent)
	out.succeeded(t, sent, fee)
}

func TestTradeSteps_bridgeOutMovesSolanaUSDCInTransitThenLandsEVMCashLessTheFee(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	out := newBridgeRun(t, f, cabal, "bridge_out")

	out.handle(t, out.event("pending", "", 10_000_000, 0))
	wantBridgePositions(t, f, position{Asset: usdcMint, Units: "100000000", Cost: "100000000"})
	wantInTransit(t, f, cabal)

	out.handle(t, out.event("submitted", "privy-1", 10_000_000, 0))
	wantBridgePositions(t, f, position{Asset: usdcMint, Units: "90000000", Cost: "90000000"})
	wantInTransit(t, f, cabal, inTransit{Swap: out.swap, Network: "base", Micros: "10000000"})

	out.handle(t, out.event("succeeded", "privy-1", 10_000_000, 7_500))
	wantBridgePositions(t, f,
		position{Asset: usdcMint, Units: "90000000", Cost: "90000000"},
		position{Asset: baseCash, Units: "9992500", Cost: "9992500"},
	)
	wantInTransit(t, f, cabal)
	if err := f.postCabal(evmBuy(t, f, cabal, 9_992_500, 400_000_000)); err != nil {
		t.Fatal(err)
	}
	wantBridgePositions(t, f,
		position{Asset: usdcMint, Units: "90000000", Cost: "90000000"},
		position{Asset: baseCash, Units: "0", Cost: "0"},
		position{Asset: baseAAPL, Units: "400000000", Cost: "9992500"},
	)
	if drift := f.drift(t); len(drift) != 0 {
		t.Fatalf("drift = %q", drift)
	}
}

func evmBuy(t *testing.T, f fixture, cabal ids.CabalID, micros, units int64) domain.CabalTxn {
	t.Helper()
	return swapTxn(t, f, cabal, baseCash, micros, baseAAPL, units)
}

func swapTxn(
	t *testing.T, f fixture, cabal ids.CabalID, paid string, micros int64, got string, units int64,
) domain.CabalTxn {
	t.Helper()
	leg := func(account domain.CabalAccount, asset string, v int64) domain.CabalEntry {
		return domain.CabalEntry{Account: account, Asset: domain.Asset(asset), Amount: money.SignedMicrosFromInt64(v)}
	}
	txn, err := domain.NewCabalTxn(domain.CabalTxnHeader{
		ID: f.ids.NewV7(), CabalID: cabal, Kind: domain.CabalSwap, Status: domain.TxnSettled, SwapID: f.ids.NewV7(),
	}, []domain.CabalEntry{
		leg(domain.CabalTreasury, paid, -micros), leg(domain.CabalVenue, paid, micros),
		leg(domain.CabalVenue, got, -units), leg(domain.CabalTreasury, got, units),
	})
	if err != nil {
		t.Fatal(err)
	}
	return txn
}

func TestTradeSteps_aBridgeOutThatFailsAfterSubmitRestoresTheSolanaUSDCExactly(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	out := newBridgeRun(t, f, cabal, "bridge_out")
	out.submitted(t, 33_333_333)

	out.failed(t, 33_333_333)

	wantBridgePositions(t, f, position{Asset: usdcMint, Units: "100000000", Cost: "100000000"})
	wantInTransit(t, f, cabal)
	if drift := f.drift(t); len(drift) != 0 {
		t.Fatalf("drift = %q", drift)
	}
}

func TestTradeSteps_aBridgeThatFailsBeforeSubmitPostsNothing(t *testing.T) {
	t.Parallel()
	for _, step := range []string{"bridge_out", "bridge_back"} {
		t.Run(step, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			cabal := f.funded(t)
			r := newBridgeRun(t, f, cabal, step)
			before := f.count(t, "cabal_txns")

			r.handle(t, r.event("pending", "", 10_000_000, 0))
			r.handle(t, r.event("failed", "", 10_000_000, 0))

			if got := f.count(t, "cabal_txns"); got != before {
				t.Fatalf("cabal txns = %d, want %d", got, before)
			}
			wantBridgePositions(t, f, position{Asset: usdcMint, Units: "100000000", Cost: "100000000"})
			wantInTransit(t, f, cabal)
		})
	}
}

func TestTradeSteps_bridgeBackMovesEVMCashInTransitThenLandsSolanaUSDCLessTheFee(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	f.bridgedOut(t, cabal, 20_000_000, 0)
	back := newBridgeRun(t, f, cabal, "bridge_back")

	back.submitted(t, 15_000_000)
	wantBridgePositions(t, f,
		position{Asset: usdcMint, Units: "80000000", Cost: "80000000"},
		position{Asset: baseCash, Units: "5000000", Cost: "5000000"},
	)
	wantInTransit(t, f, cabal, inTransit{Swap: back.swap, Network: "base", Micros: "15000000"})

	back.succeeded(t, 15_000_000, 2_500)
	wantBridgePositions(t, f,
		position{Asset: usdcMint, Units: "94997500", Cost: "94997500"},
		position{Asset: baseCash, Units: "5000000", Cost: "5000000"},
	)
	wantInTransit(t, f, cabal)
}

func TestTradeSteps_aBridgeBackThatFailsAfterSubmitRestoresTheEVMCash(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	f.bridgedOut(t, cabal, 20_000_000, 10_000)
	back := newBridgeRun(t, f, cabal, "bridge_back")
	back.submitted(t, 19_990_000)

	back.failed(t, 19_990_000)

	wantBridgePositions(t, f,
		position{Asset: usdcMint, Units: "80000000", Cost: "80000000"},
		position{Asset: baseCash, Units: "19990000", Cost: "19990000"},
	)
	wantInTransit(t, f, cabal)
}

func TestTradeSteps_aFinishArrivingBeforeItsSubmitEndsWhereInOrderDeliveryEnds(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		finish func(t *testing.T, r bridgeRun)
		want   []position
	}{
		"succeeded": {
			finish: func(t *testing.T, r bridgeRun) {
				t.Helper()
				r.succeeded(t, 10_000_000, 7_500)
			},
			want: []position{
				{Asset: usdcMint, Units: "90000000", Cost: "90000000"},
				{Asset: baseCash, Units: "9992500", Cost: "9992500"},
			},
		},
		"failed": {
			finish: func(t *testing.T, r bridgeRun) {
				t.Helper()
				r.failed(t, 10_000_000)
			},
			want: []position{{Asset: usdcMint, Units: "100000000", Cost: "100000000"}},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			cabal := f.funded(t)
			out := newBridgeRun(t, f, cabal, "bridge_out")

			tc.finish(t, out)
			out.submitted(t, 10_000_000)

			wantBridgePositions(t, f, tc.want...)
			wantInTransit(t, f, cabal)
		})
	}
}

func TestTradeSteps_swapStepsPostNothingAndANonEVMBridgeIsRefused(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	before := f.count(t, "cabal_txns")
	swap := newBridgeRun(t, f, cabal, "swap")
	for _, status := range []string{"pending", "submitted", "succeeded", "failed"} {
		swap.handle(t, swap.event(status, "privy-swap", 10_000_000, 0))
	}
	if got := f.count(t, "cabal_txns"); got != before {
		t.Fatalf("cabal txns = %d after swap steps, want %d", got, before)
	}

	out := newBridgeRun(t, f, cabal, "bridge_out")
	e := out.event("submitted", "privy-1", 10_000_000, 0)
	e.Network = chain.NetworkSolana
	wantCode(t, f.stepChanged(e), errs.CodeInvalidInput)
	e = out.event("landed", "privy-1", 10_000_000, 0)
	wantCode(t, f.stepChanged(e), errs.CodeInvalidInput)
	wantBridgePositions(t, f, position{Asset: usdcMint, Units: "100000000", Cost: "100000000"})
	wantInTransit(t, f, cabal)
}

func TestLedgerCheck_catchesDriftOnEVMCashAndStockRows(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	f.bridgedOut(t, cabal, 10_000_000, 7_500)
	if err := f.postCabal(evmBuy(t, f, cabal, 5_000_000, 200_000_000)); err != nil {
		t.Fatal(err)
	}
	wantBridgePositions(t, f,
		position{Asset: usdcMint, Units: "90000000", Cost: "90000000"},
		position{Asset: baseCash, Units: "4992500", Cost: "4992500"},
		position{Asset: baseAAPL, Units: "200000000", Cost: "5000000"},
	)
	if drift := f.drift(t); len(drift) != 0 {
		t.Fatalf("drift before the plants = %q", drift)
	}
	for _, plant := range []struct{ sql, asset string }{
		{`UPDATE cabal_positions SET units = units + 1, cost_basis_micros = cost_basis_micros - 1 WHERE asset = $1`, baseCash},
		{`UPDATE cabal_positions SET cost_basis_micros = cost_basis_micros + 1 WHERE asset = $1`, baseAAPL},
	} {
		if _, err := f.pool.Exec(t.Context(), plant.sql, plant.asset); err != nil {
			t.Fatal(err)
		}
	}
	prefix := "cabal_positions " + cabal.String() + " "
	want := []string{
		prefix + baseCash + ": entries 4992500, position 4992501",
		prefix + baseCash + ": cost basis entries 4992500, position 4992499",
		prefix + baseAAPL + ": cost basis entries 5000000, position 5000001",
	}
	if drift := f.drift(t); !slices.Equal(drift, want) {
		t.Fatalf("drift =\n%q\nwant\n%q", drift, want)
	}
}

func TestLedgerCheck_aTokenAtAnotherNetworksCashAddressIsCostedAsAPurchase(t *testing.T) {
	t.Parallel()
	const lookalike = "arbitrum:0x833589fcd6edb6e08f4c7c32d4f71b54bda02913"
	f := newFixture(t)
	cabal := f.funded(t)
	if err := f.postCabal(swapTxn(t, f, cabal, usdcMint, 7_000_000, lookalike, 9_000_000)); err != nil {
		t.Fatal(err)
	}
	if drift := f.drift(t); len(drift) != 0 {
		t.Fatalf("drift = %q", drift)
	}
	if _, err := f.pool.Exec(t.Context(),
		`UPDATE cabal_positions SET cost_basis_micros = units WHERE asset = $1`, lookalike); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"cabal_positions " + cabal.String() + " " + lookalike + ": cost basis entries 7000000, position 9000000",
	}
	if drift := f.drift(t); !slices.Equal(drift, want) {
		t.Fatalf("drift = %q, want %q", drift, want)
	}
}

func TestTradeSteps_refusesBridgeAmountsTheLedgerCannotPostAndWritesNothing(t *testing.T) {
	t.Parallel()
	const pastInt64 = 1<<63 + 1
	f := newFixture(t)
	cabal := f.funded(t)
	out := newBridgeRun(t, f, cabal, "bridge_out")
	out.submitted(t, 10_000_000)
	before := f.count(t, "cabal_txns")
	huge := newBridgeRun(t, f, cabal, "bridge_out")

	for name, e := range map[string]events.TradeStepChanged{
		"submitted past int64": huge.event("submitted", "privy-huge", pastInt64, 0),
		"succeeded past int64": huge.event("succeeded", "privy-huge", pastInt64, 0),
		"fee above sent":       out.event("succeeded", "privy-1", 10_000_000, 10_000_001),
	} {
		wantCode(t, f.stepChanged(e), errs.CodeInvalidInput)
		if got := f.count(t, "cabal_txns"); got != before {
			t.Fatalf("%s: cabal txns = %d, want %d", name, got, before)
		}
	}
	wantBridgePositions(t, f, position{Asset: usdcMint, Units: "90000000", Cost: "90000000"})
	wantInTransit(t, f, cabal, inTransit{Swap: out.swap, Network: "base", Micros: "10000000"})
}

func TestTradeSteps_aDatabaseErrorFailsTheDeliveryAndPostsNothing(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	run := newBridgeRun(t, f, cabal, "bridge_out")
	before := f.count(t, "cabal_txns")
	h := adapters.TradeSteps{Ledger: f.ledger, Hints: &hints{}}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()

	for _, status := range []string{"submitted", "succeeded"} {
		e := run.event(status, "privy-"+run.swap.String(), 10_000_000, 0)
		err := f.do(func(_ context.Context, tx db.Tx) error { return h.Handle(cancelled, tx, e, confirmedAt()) })
		if err == nil {
			t.Fatalf("%s on a cancelled context: err = nil, want an error", status)
		}
	}
	if got := f.count(t, "cabal_txns"); got != before {
		t.Fatalf("cabal txns = %d, want %d", got, before)
	}
	wantInTransit(t, f, cabal)
}
