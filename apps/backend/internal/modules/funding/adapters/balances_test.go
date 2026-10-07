package adapters_test

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

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

func (r balanceRPC) TokenBalance(context.Context, chain.SolanaAddress, chain.Mint) (money.BaseUnits, error) {
	return r.amount, r.err
}

type balanceOutflows struct {
	amount money.Micros
	err    error
}

func (o balanceOutflows) InFlightMicros(context.Context, ids.UserID) (money.Micros, error) {
	return o.amount, o.err
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

func (r *flakyRPC) TokenBalance(context.Context, chain.SolanaAddress, chain.Mint) (money.BaseUnits, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.amount, r.err
}

type mutableOutflows struct {
	mu     sync.Mutex
	amount money.Micros
	err    error
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

type displayRig struct {
	b     *adapters.Balances
	rpc   *flakyRPC
	funds *mutableOutflows
	clock *testkit.Clock
	user  ids.UserID
	start time.Time
}

func newDisplayRig() displayRig {
	rpc := &flakyRPC{amount: money.NewBaseUnits(10, 6)}
	funds := &mutableOutflows{}
	start := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	clk := testkit.NewClock(start)
	return displayRig{
		b: adapters.NewBalances(balanceWallets{address: "wallet"}, func() adapters.TokenBalances { return rpc },
			adapters.Outflows{Funds: funds, Withdrawals: balanceOutflows{amount: money.MicrosFromUint64(2)}},
			clk, chain.Mint{Address: "usdc", Decimals: 6}),
		rpc: rpc, funds: funds, clock: clk, user: ids.UserIDFrom(ids.Real{}.NewV7()), start: start,
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
		if _, err := r.b.ForDisplay(t.Context(), r.user); errs.CodeOf(err) != errs.CodeInternal {
			t.Fatalf("ForDisplay error = %v", err)
		}
	})
	t.Run("in-flight read fails on the cached path", func(t *testing.T) {
		t.Parallel()
		r := newDisplayRig()
		if _, err := r.b.Available(t.Context(), r.user); err != nil {
			t.Fatalf("Available error = %v", err)
		}
		r.rpc.fail(down)
		r.funds.set(money.Micros{}, errs.New(errs.CodeInternal, "test"))
		if _, err := r.b.ForDisplay(t.Context(), r.user); errs.CodeOf(err) != errs.CodeInternal {
			t.Fatalf("ForDisplay error = %v", err)
		}
	})
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
