package adapters_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type observeRig struct {
	b     *adapters.Balances
	rpc   *landingRPC
	clock *testkit.Clock
	user  ids.UserID
}

func newObserveRig(onChain uint64, before, within balanceOutflows) observeRig {
	rpc := &landingRPC{onChain: onChain, slot: 10, statuses: map[chain.Signature]solana.Status{}}
	clk := testkit.NewClock(time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC))
	inTx := func(dbsqlc.DBTX) app.Outflows { return within }
	b := adapters.NewBalances(balanceWallets{address: "wallet"}, func() adapters.ChainReads { return rpc },
		adapters.Outflows{
			Funds: before, Withdrawals: balanceOutflows{}, FundsOn: inTx,
			WithdrawalsOn: func(dbsqlc.DBTX) app.Outflows { return balanceOutflows{} },
		},
		clk, chain.Mint{Address: "usdc", Decimals: 6})
	return observeRig{b: b, rpc: rpc, clock: clk, user: ids.UserIDFrom(ids.Real{}.NewV7())}
}

func TestBalance_AvailableInMatchesAvailableWhenNothingMoved(t *testing.T) {
	t.Parallel()
	const usd = 1_000_000
	r := newObserveRig(100*usd, outflow("sig", 40), outflow("sig", 40))
	obs, err := r.b.Observe(t.Context(), r.user)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.b.AvailableIn(t.Context(), nil, obs)
	want, wantErr := r.b.Available(t.Context(), r.user)
	if err != nil || wantErr != nil || got.AvailableMicros != want.AvailableMicros ||
		got.AvailableMicros.Uint64() != 60*usd {
		t.Fatalf("AvailableIn = %+v, %v, want %+v, %v", got, err, want, wantErr)
	}
}

func TestBalance_AvailableInRefusesAnOutflowThatSettledAfterTheReading(t *testing.T) {
	t.Parallel()
	const usd = 1_000_000
	r := newObserveRig(100*usd, outflow("sig", 40), balanceOutflows{})
	obs, err := r.b.Observe(t.Context(), r.user)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.b.AvailableIn(t.Context(), nil, obs); errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("AvailableIn code = %q, err %v, want %q", errs.CodeOf(err), err, errs.CodeRPCUnavailable)
	}
}

func TestBalance_AvailableInAcceptsAnOutflowTheReadingAlreadyCounted(t *testing.T) {
	t.Parallel()
	const usd = 1_000_000
	r := newObserveRig(60*usd, outflow("sig", 40), balanceOutflows{})
	r.rpc.statuses["sig"] = solana.Status{State: solana.StateFinalized, Slot: 9}
	obs, err := r.b.Observe(t.Context(), r.user)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.b.AvailableIn(t.Context(), nil, obs)
	if err != nil || got.AvailableMicros.Uint64() != 60*usd {
		t.Fatalf("AvailableIn = %+v, %v, want 60 available", got, err)
	}
}

func TestBalance_AvailableInCountsAnOutflowSubmittedAfterTheReading(t *testing.T) {
	t.Parallel()
	const usd = 1_000_000
	r := newObserveRig(100*usd, balanceOutflows{}, outflow("late", 30))
	obs, err := r.b.Observe(t.Context(), r.user)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.b.AvailableIn(t.Context(), nil, obs)
	if err != nil || got.AvailableMicros.Uint64() != 70*usd {
		t.Fatalf("AvailableIn = %+v, %v, want 70 available", got, err)
	}
}

func TestBalance_AvailableInRefusesAReadingOlderThanItsWindow(t *testing.T) {
	t.Parallel()
	r := newObserveRig(100, balanceOutflows{}, balanceOutflows{})
	obs, err := r.b.Observe(t.Context(), r.user)
	if err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(6 * time.Second)
	if _, err := r.b.AvailableIn(t.Context(), nil, obs); errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("AvailableIn code = %q, err %v, want %q", errs.CodeOf(err), err, errs.CodeRPCUnavailable)
	}
	r.clock.Advance(-3 * time.Second)
	if _, err := r.b.AvailableIn(t.Context(), nil, obs); err != nil {
		t.Fatalf("AvailableIn inside the window error = %v", err)
	}
}

func TestBalance_AvailableInReportsATransactionReadFailure(t *testing.T) {
	t.Parallel()
	r := newObserveRig(100, balanceOutflows{}, balanceOutflows{err: errs.New(errs.CodeInternal, "test")})
	obs, err := r.b.Observe(t.Context(), r.user)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.b.AvailableIn(t.Context(), nil, obs); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("AvailableIn code = %q, err %v, want %q", errs.CodeOf(err), err, errs.CodeInternal)
	}
}
