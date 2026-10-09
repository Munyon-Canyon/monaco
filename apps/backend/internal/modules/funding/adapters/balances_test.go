package adapters_test

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/fundingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type balanceWallets struct {
	address chain.SolanaAddress
	err     error
}

type fixedBalances struct {
	balance port.Balance
	err     error
}

func (b fixedBalances) ForDisplay(context.Context, ids.UserID) (port.Balance, error) {
	return b.balance, b.err
}

func TestHTTPGetMyBalance(t *testing.T) {
	t.Parallel()
	user := ids.UserIDFrom(ids.Real{}.NewV7())
	now := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	h := adapters.HTTP{
		Balances: fixedBalances{balance: port.Balance{
			OnChainMicros:            money.MicrosFromUint64(5),
			InFlightFundMicros:       money.MicrosFromUint64(1),
			InFlightWithdrawalMicros: money.MicrosFromUint64(2),
			AvailableMicros:          money.MicrosFromUint64(2),
			AsOf:                     now,
		}},
		Wallets: balanceWallets{address: "wallet"},
	}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: user.String()})
	got, err := h.GetMyBalance(ctx, api.GetMyBalanceRequestObject{})
	if err != nil || got.(api.GetMyBalance200JSONResponse).AvailableMicros != "2" ||
		got.(api.GetMyBalance200JSONResponse).InFlightMicros != "3" {
		t.Fatalf("GetMyBalance = %#v, %v", got, err)
	}
	for name, ctx := range map[string]context.Context{
		"missing": t.Context(),
		"wrong":   auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorSystem, ID: user.String()}),
		"invalid": auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: "bad"}),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := h.GetMyBalance(ctx, api.GetMyBalanceRequestObject{}); err == nil {
				t.Fatal("GetMyBalance error = nil")
			}
		})
	}
	for name, h := range map[string]adapters.HTTP{
		"balance": {Balances: fixedBalances{err: errs.New(errs.CodeInternal, "test")}, Wallets: balanceWallets{}},
		"wallet":  {Balances: fixedBalances{}, Wallets: balanceWallets{err: errs.New(errs.CodeInternal, "test")}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := h.GetMyBalance(ctx, api.GetMyBalanceRequestObject{}); err == nil {
				t.Fatal("GetMyBalance error = nil")
			}
		})
	}
	if _, err := (adapters.HTTP{Balances: fixedBalances{balance: port.Balance{
		InFlightFundMicros: money.MicrosFromUint64(math.MaxUint64), InFlightWithdrawalMicros: money.MicrosFromUint64(1),
	}}, Wallets: balanceWallets{}}).GetMyBalance(ctx, api.GetMyBalanceRequestObject{}); err == nil {
		t.Fatal("GetMyBalance with overflowing in-flight balance error = nil")
	}
}

func (w balanceWallets) MemberWalletAddress(context.Context, ids.UserID) (chain.SolanaAddress, error) {
	return w.address, w.err
}

type balanceRPC struct {
	amount money.BaseUnits
	err    error
}

func (r balanceRPC) TokenBalanceAt(context.Context, chain.SolanaAddress, chain.Mint, string) (money.BaseUnits, error) {
	return r.amount, r.err
}

type balanceOutflows struct {
	amount money.Micros
	err    error
}

func (o balanceOutflows) InFlightMicros(context.Context, ids.UserID) (money.Micros, error) {
	return o.amount, o.err
}

func (o balanceOutflows) LastChange(context.Context, ids.UserID) (time.Time, error) {
	return time.Time{}, nil
}

func TestBalancesAvailableReturnsDependenciesErrors(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test")
	user := ids.UserIDFrom(ids.Real{}.NewV7())
	newBalances := func(wallets balanceWallets, rpc balanceRPC, outflows balanceOutflows) *adapters.Balances {
		return adapters.NewBalances(wallets, func() adapters.TokenBalances { return rpc },
			adapters.Outflows{Funds: outflows, Withdrawals: balanceOutflows{}},
			testkit.NewClock(time.Time{}), chain.Mint{Decimals: 6})
	}
	for name, b := range map[string]*adapters.Balances{
		"wallet":   newBalances(balanceWallets{err: boom}, balanceRPC{}, balanceOutflows{}),
		"rpc":      newBalances(balanceWallets{}, balanceRPC{err: boom}, balanceOutflows{}),
		"outflow":  newBalances(balanceWallets{}, balanceRPC{amount: money.NewBaseUnits(1, 6)}, balanceOutflows{err: boom}),
		"decimals": newBalances(balanceWallets{}, balanceRPC{amount: money.NewBaseUnits(1, 5)}, balanceOutflows{}),
		"withdrawals": adapters.NewBalances(balanceWallets{},
			func() adapters.TokenBalances { return balanceRPC{amount: money.NewBaseUnits(1, 6)} },
			adapters.Outflows{Funds: balanceOutflows{}, Withdrawals: balanceOutflows{err: boom}},
			testkit.NewClock(time.Time{}), chain.Mint{Decimals: 6}),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := b.Available(t.Context(), user); err == nil {
				t.Fatal("Available error = nil")
			}
		})
	}
}

func TestBalance_AvailableSubtractsInFlight(t *testing.T) {
	t.Parallel()
	user := ids.UserIDFrom(ids.Real{}.NewV7())
	now := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	b := adapters.NewBalances(
		balanceWallets{address: "wallet"},
		func() adapters.TokenBalances { return balanceRPC{amount: money.NewBaseUnits(10, 6)} },
		adapters.Outflows{
			Funds: balanceOutflows{
				amount: money.MicrosFromUint64(3),
			},
			Withdrawals: balanceOutflows{amount: money.MicrosFromUint64(2)},
		},
		testkit.NewClock(now), chain.Mint{Address: "usdc", Decimals: 6},
	)
	got, err := b.Available(t.Context(), user)
	if err != nil || got.OnChainMicros.String() != "10" || got.InFlightFundMicros.String() != "3" ||
		got.InFlightWithdrawalMicros.String() != "2" || got.AvailableMicros.String() != "5" || !got.AsOf.Equal(now) {
		t.Fatalf("Available = %+v, %v", got, err)
	}
}

func TestBalance_AvailableClampsAtZero(t *testing.T) {
	t.Parallel()
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
	user := ids.UserIDFrom(ids.Real{}.NewV7())
	b := adapters.NewBalances(
		balanceWallets{},
		func() adapters.TokenBalances { return balanceRPC{amount: money.NewBaseUnits(3, 6)} },
		adapters.Outflows{Funds: balanceOutflows{amount: money.MicrosFromUint64(10)}, Withdrawals: balanceOutflows{}},
		testkit.NewClock(time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)), chain.Mint{Address: "usdc", Decimals: 6},
	)
	got, err := b.Available(ctx, user)
	if err != nil || !got.AvailableMicros.IsZero() {
		t.Fatalf("Available = %+v, %v", got, err)
	}
	withdrawing := adapters.NewBalances(
		balanceWallets{},
		func() adapters.TokenBalances { return balanceRPC{amount: money.NewBaseUnits(3, 6)} },
		adapters.Outflows{
			Funds:       balanceOutflows{amount: money.MicrosFromUint64(1)},
			Withdrawals: balanceOutflows{amount: money.MicrosFromUint64(4)},
		},
		testkit.NewClock(time.Time{}), chain.Mint{Address: "usdc", Decimals: 6},
	)
	if got, err := withdrawing.Available(t.Context(), user); err != nil || !got.AvailableMicros.IsZero() {
		t.Fatalf("Available with withdrawals = %+v, %v", got, err)
	}
	var line map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(logs.Bytes()))), &line); err != nil ||
		line["level"] != "WARN" || line["msg"] != "funding.balance.clamped" || line["user_id"] != user.String() {
		t.Fatalf("clamp log = %s, %v", logs.Bytes(), err)
	}
}

type flakyRPC struct {
	mu     sync.Mutex
	amount money.BaseUnits
	err    error
}

func (r *flakyRPC) fail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
}

func (r *flakyRPC) TokenBalanceAt(context.Context, chain.SolanaAddress, chain.Mint, string) (money.BaseUnits, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.amount, r.err
}

type mutableOutflows struct {
	mu        sync.Mutex
	amount    money.Micros
	err       error
	changed   time.Time
	changeErr error
}

func (o *mutableOutflows) set(amount money.Micros, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.amount, o.err = amount, err
}

func (o *mutableOutflows) InFlightMicros(context.Context, ids.UserID) (money.Micros, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.amount, o.err
}

func (o *mutableOutflows) change(at time.Time, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.changed, o.changeErr = at, err
}

func (o *mutableOutflows) LastChange(context.Context, ids.UserID) (time.Time, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.changed, o.changeErr
}

type displayRig struct {
	b           *adapters.Balances
	rpc         *flakyRPC
	funds       *mutableOutflows
	withdrawals *mutableOutflows
	clock       *testkit.Clock
	user        ids.UserID
	start       time.Time
}

func newDisplayRig() displayRig {
	rpc := &flakyRPC{amount: money.NewBaseUnits(10, 6)}
	funds, withdrawals := &mutableOutflows{}, &mutableOutflows{amount: money.MicrosFromUint64(2)}
	start := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	clk := testkit.NewClock(start)
	return displayRig{
		b: adapters.NewBalances(balanceWallets{address: "wallet"}, func() adapters.TokenBalances { return rpc },
			adapters.Outflows{Funds: funds, Withdrawals: withdrawals},
			clk, chain.Mint{Address: "usdc", Decimals: 6}),
		rpc: rpc, funds: funds, withdrawals: withdrawals, clock: clk, user: ids.UserIDFrom(ids.Real{}.NewV7()),
		start: start,
	}
}

func TestBalance_ForDisplayServesLastReadingWhenRPCUnavailable(t *testing.T) {
	t.Parallel()
	r := newDisplayRig()
	if _, err := r.b.ForDisplay(t.Context(), r.user); err != nil {
		t.Fatalf("live ForDisplay error = %v", err)
	}
	r.clock.Advance(9 * time.Minute)
	r.rpc.fail(errs.New(errs.CodeRPCUnavailable, "test"))
	r.funds.set(money.MicrosFromUint64(3), nil)
	got, err := r.b.ForDisplay(t.Context(), r.user)
	if err != nil || got.OnChainMicros.String() != "10" || got.InFlightFundMicros.String() != "3" ||
		got.InFlightWithdrawalMicros.String() != "2" || got.AvailableMicros.String() != "5" || !got.AsOf.Equal(r.start) {
		t.Fatalf("ForDisplay = %+v, %v", got, err)
	}
}

func TestBalance_ForDisplayWithoutFreshReadingFails(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeRPCUnavailable, "test")
	t.Run("no reading", func(t *testing.T) {
		t.Parallel()
		r := newDisplayRig()
		r.rpc.fail(down)
		if _, err := r.b.ForDisplay(t.Context(), r.user); errs.CodeOf(err) != errs.CodeRPCUnavailable {
			t.Fatalf("ForDisplay error = %v", err)
		}
	})
	t.Run("reading too old", func(t *testing.T) {
		t.Parallel()
		r := newDisplayRig()
		if _, err := r.b.Available(t.Context(), r.user); err != nil {
			t.Fatalf("Available error = %v", err)
		}
		r.clock.Advance(10 * time.Minute)
		r.rpc.fail(down)
		if _, err := r.b.ForDisplay(t.Context(), r.user); errs.CodeOf(err) != errs.CodeRPCUnavailable {
			t.Fatalf("ForDisplay error = %v", err)
		}
	})
	t.Run("other error is unchanged", func(t *testing.T) {
		t.Parallel()
		r := newDisplayRig()
		if _, err := r.b.Available(t.Context(), r.user); err != nil {
			t.Fatalf("Available error = %v", err)
		}
		r.rpc.fail(errs.New(errs.CodeInternal, "test"))
		if _, err := r.b.ForDisplay(t.Context(), r.user); err == nil || errs.CodeOf(err) != errs.CodeInternal {
			t.Fatalf("ForDisplay error = %v", err)
		}
	})
}

func TestBalance_ForDisplayReturnsReadErrorsOnTheCachedPath(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test")
	for name, breakRead := range map[string]func(displayRig){
		"last change": func(r displayRig) { r.withdrawals.change(time.Time{}, boom) },
		"in-flight":   func(r displayRig) { r.funds.set(money.Micros{}, boom) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newDisplayRig()
			if _, err := r.b.Available(t.Context(), r.user); err != nil {
				t.Fatalf("Available error = %v", err)
			}
			r.rpc.fail(errs.New(errs.CodeRPCUnavailable, "test"))
			breakRead(r)
			if _, err := r.b.ForDisplay(t.Context(), r.user); err == nil || errs.CodeOf(err) != errs.CodeInternal {
				t.Fatalf("ForDisplay error = %v, want internal", err)
			}
		})
	}
}

func TestBalance_AvailableIgnoresCachedReading(t *testing.T) {
	t.Parallel()
	r := newDisplayRig()
	if _, err := r.b.Available(t.Context(), r.user); err != nil {
		t.Fatalf("Available error = %v", err)
	}
	r.rpc.fail(errs.New(errs.CodeRPCUnavailable, "test"))
	if _, err := r.b.Available(t.Context(), r.user); errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("Available error = %v, want rpc_unavailable", err)
	}
}

func TestBalance_ForDisplayServesTheReadingOnlyWhenNoOutflowMovedSince(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		withdrawal bool
		since      time.Duration
		served     bool
	}{
		"fund moved before the reading":       {since: -time.Minute, served: true},
		"fund moved at the reading":           {},
		"fund moved after the reading":        {since: time.Minute},
		"withdrawal moved before the reading": {withdrawal: true, since: -time.Minute, served: true},
		"withdrawal moved at the reading":     {withdrawal: true},
		"withdrawal moved after the reading":  {withdrawal: true, since: time.Minute},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newDisplayRig()
			if _, err := r.b.ForDisplay(t.Context(), r.user); err != nil {
				t.Fatalf("live ForDisplay error = %v", err)
			}
			moved := r.funds
			if tc.withdrawal {
				moved = r.withdrawals
			}
			moved.change(r.start.Add(tc.since), nil)
			r.clock.Advance(2 * time.Minute)
			r.rpc.fail(errs.New(errs.CodeRPCUnavailable, "test"))
			got, err := r.b.ForDisplay(t.Context(), r.user)
			served := err == nil && got.AsOf.Equal(r.start) && got.AvailableMicros.String() == "8"
			if served != tc.served || (!served && errs.CodeOf(err) != errs.CodeRPCUnavailable) {
				t.Fatalf("ForDisplay = %+v, %v, want the cached reading served = %t", got, err, tc.served)
			}
		})
	}
}

type commitmentRPC map[string]money.BaseUnits

func (r commitmentRPC) TokenBalanceAt(
	_ context.Context, _ chain.SolanaAddress, _ chain.Mint, commitment string,
) (money.BaseUnits, error) {
	return r[commitment], nil
}

func settlingBalances(confirmed, finalized, fund, withdrawal uint64) *adapters.Balances {
	rpc := commitmentRPC{"confirmed": money.NewBaseUnits(confirmed, 6), "finalized": money.NewBaseUnits(finalized, 6)}
	outflows := adapters.Outflows{
		Funds:       balanceOutflows{amount: money.MicrosFromUint64(fund)},
		Withdrawals: balanceOutflows{amount: money.MicrosFromUint64(withdrawal)},
	}
	clk := testkit.NewClock(time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC))
	return adapters.NewBalances(balanceWallets{address: "wallet"}, func() adapters.TokenBalances { return rpc },
		outflows, clk, chain.Mint{Address: "usdc", Decimals: 6})
}

func TestBalance_AvailableSubtractsALandedOutflowOnce(t *testing.T) {
	t.Parallel()
	const usd = 1_000_000
	for name, tc := range map[string]struct{ confirmed, fund, withdrawal, want uint64 }{
		"fund 40 of 100":           {confirmed: 60, fund: 40, want: 60},
		"withdraw 40 of 100":       {confirmed: 60, withdrawal: 40, want: 60},
		"max withdrawal of 100":    {confirmed: 0, withdrawal: 100, want: 0},
		"fund 40 then withdraw 60": {confirmed: 0, fund: 40, withdrawal: 60, want: 0},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			logs := &testkit.Logs{}
			b := settlingBalances(tc.confirmed*usd, 100*usd, tc.fund*usd, tc.withdrawal*usd)
			got, err := b.Available(withLogs(t.Context(), logs), ids.UserIDFrom(ids.Real{}.NewV7()))
			if err != nil || got.OnChainMicros.Uint64() != 100*usd || got.AvailableMicros.Uint64() != tc.want*usd {
				t.Fatalf("Available = %+v, %v, want 100 on chain and %d available", got, err, tc.want)
			}
			if len(logs.Bytes()) != 0 {
				t.Fatalf("logs = %s, want no funding.balance.clamped while the outflow settles", logs.Bytes())
			}
		})
	}
}

func TestBalance_AvailableCountsEachOutflowOnceWhateverHasLanded(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		finalized := rapid.Uint64Range(0, 1<<40).Draw(rt, "finalized")
		fund := rapid.Uint64Range(0, finalized).Draw(rt, "fund")
		withdrawal := rapid.Uint64Range(0, finalized-fund).Draw(rt, "withdrawal")
		confirmed := rapid.Uint64Range(finalized-fund-withdrawal, finalized).Draw(rt, "confirmed")
		logs := &testkit.Logs{}
		got, err := settlingBalances(confirmed, finalized, fund, withdrawal).
			Available(withLogs(t.Context(), logs), ids.UserIDFrom(ids.Real{}.NewV7()))
		if err != nil || got.AvailableMicros.Uint64() != finalized-fund-withdrawal || len(logs.Bytes()) != 0 {
			rt.Fatalf("Available = %+v, %v, logs %s, want %d with no clamp", got, err, logs.Bytes(),
				finalized-fund-withdrawal)
		}
	})
}

func withLogs(ctx context.Context, logs *testkit.Logs) context.Context {
	return observability.WithLogger(ctx, observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
}
