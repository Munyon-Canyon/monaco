package treasury_test

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestModule_pollsFundsAndSweepsCashOuts(t *testing.T) {
	t.Parallel()
	pollers := treasury.New(module.Deps{}).Pollers()
	names := make([]string, 0, len(pollers))
	for _, p := range pollers {
		names = append(names, p.Name())
	}
	if want := []string{"treasury.fund-transfers", "treasury.cashout-sweeper"}; !slices.Equal(names, want) {
		t.Fatalf("pollers = %v, want %v", names, want)
	}
}

func TestModule_servesActivityAndConsumesTradeEvents(t *testing.T) {
	t.Parallel()
	m := treasury.New(module.Deps{})
	if m.Name() != "treasury" || !testkit.Serves(m.Mount, "GET", "/v1/me/txns") ||
		!testkit.Serves(m.Mount, "GET", "/v1/cabals/c/activity") || testkit.Serves(m.Mount, "GET", "/v1/cabals") ||
		!testkit.Serves(m.Mount, "GET", "/v1/fund-transfers/f") {
		t.Fatalf("module = %s, pollers %v", m.Name(), m.Pollers())
	}
	var got []string
	for _, c := range m.Consumers() {
		for _, h := range c.Handlers {
			got = append(got, c.Durable+" "+h.Name+" "+string(h.Type()))
		}
	}
	want := []string{
		"treasury_trades treasury.trades " + string(events.TypeTradeConfirmed),
		"treasury_activity treasury.activity.submitted " + string(events.TypeTradeSubmitted),
		"treasury_activity treasury.activity.confirmed " + string(events.TypeTradeConfirmed),
		"treasury_activity treasury.activity.failed " + string(events.TypeTradeFailed),
		"treasury_activity treasury.activity.fund_submitted " + string(events.TypeFundSubmitted),
		"treasury_activity treasury.activity.funded " + string(events.TypeFunded),
		"treasury_activity treasury.activity.fund_failed " + string(events.TypeFundFailed),
		"treasury_cashout treasury.cashout " + string(events.TypeCashOutStarted),
		"treasury_cashout treasury.cashout.confirmed " + string(events.TypeTradeConfirmed),
		"treasury_cashout treasury.cashout.failed " + string(events.TypeTradeFailed),
		"treasury_cashout_payout treasury.cashout_payout " + string(events.TypeCashOutStarted),
		"treasury_user_ledger treasury.user_ledger " + string(events.TypeDepositCredited),
		"treasury_user_ledger treasury.withdrawal_ledger " + string(events.TypeWithdrawalConfirmed),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("consumers = %q, want %q", got, want)
	}
	if _, ok := m.Queries().(*adapters.Queries); !ok {
		t.Fatalf("Queries() = %T, want *adapters.Queries", m.Queries())
	}
	if m.SignatureOwner() == nil || m.WalletLedger() == nil {
		t.Fatal("signature owner or wallet ledger = nil")
	}
}

type cashOutPauseModule struct{ pauses fundingport.Pauses }

func (cashOutPauseModule) Name() string { return "cash_out_pauses" }

func (cashOutPauseModule) Mount(apibase.Mount) {}

func (cashOutPauseModule) Consumers() []bus.Consumer { return nil }

func (cashOutPauseModule) Pollers() []poller.Poller { return nil }

func (m cashOutPauseModule) Pauses() fundingport.Pauses { return m.pauses }

type fixedCashOutPauses struct{ pause fundingport.Pause }

func (p fixedCashOutPauses) IsPaused(context.Context, ids.CabalID) (fundingport.Pause, error) {
	return p.pause, nil
}

func (fixedCashOutPauses) PausedCabals(context.Context) (fundingport.PausedSet, error) {
	return fundingport.PausedSet{}, nil
}

func TestModule_cashOutPausesFailClosedAndWireReasons(t *testing.T) {
	t.Parallel()
	alone := treasury.New(module.Deps{})
	if _, err := alone.CashOutPause(t.Context(), ids.CabalID{}); errs.CodeOf(err) !=
		errs.CodeUpstreamUnavailable {
		t.Fatalf("unwired pause error = %v", err)
	}
	since := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	wired := treasury.New(module.Deps{})
	module.NewSet(wired, cashOutPauseModule{fixedCashOutPauses{pause: fundingport.Pause{
		Paused: true, Reasons: append(fundingport.Pause{}.Reasons, "ops"), Since: since,
	}}})
	pause, err := wired.CashOutPause(t.Context(), ids.CabalID{})
	if err != nil || !pause.Paused || !slices.Equal(pause.Reasons, []string{"ops"}) || !pause.Since.Equal(since) {
		t.Fatalf("pause = %#v, %v", pause, err)
	}
}

func TestModule_wireTakesMembersAndUsersFromTheBuiltSetAndFailsClosedWithout(t *testing.T) {
	t.Parallel()
	d := module.Deps{}
	alone := treasury.New(d)
	module.NewSet(alone)
	members, users := alone.Reads()
	if _, ok := members.(app.UnwiredReads); !ok {
		t.Fatalf("members = %T, want app.UnwiredReads", members)
	}
	if _, ok := users.(app.UnwiredReads); !ok {
		t.Fatalf("users = %T, want app.UnwiredReads", users)
	}
	wired := treasury.New(d)
	module.NewSet(wired, cabal.New(d), identity.New(d))
	members, users = wired.Reads()
	if reflect.TypeOf(members) != reflect.TypeOf(cabal.New(d).Queries()) ||
		reflect.TypeOf(users) != reflect.TypeOf(identity.New(d).Queries()) {
		t.Fatalf("reads = %T, %T, want cabal's and identity's Queries", members, users)
	}
	treasuryWallets, memberWallets := alone.PayoutWallets()
	if _, ok := treasuryWallets.(app.UnwiredReads); !ok {
		t.Fatalf("treasury wallets = %T, want app.UnwiredReads", treasuryWallets)
	}
	if _, ok := memberWallets.(app.UnwiredReads); !ok {
		t.Fatalf("member wallets = %T, want app.UnwiredReads", memberWallets)
	}
	treasuryWallets, memberWallets = wired.PayoutWallets()
	if reflect.TypeOf(treasuryWallets) != reflect.TypeOf(cabal.New(d).Queries()) ||
		reflect.TypeOf(memberWallets) != reflect.TypeOf(identity.New(d).Queries()) {
		t.Fatalf("payout wallets = %T, %T, want cabal's and identity's Queries", treasuryWallets, memberWallets)
	}
}

func TestModule_payoutTransfersNeedPrivyAndRelayerKeys(t *testing.T) {
	t.Parallel()
	privy := config.Privy{BaseURL: "http://127.0.0.1", VerificationKey: fakes.PrivyVerificationKey()}
	relayer := config.Relayer{PrivateKey: chain.EncodeBase58(fakes.FixtureKey("relayer"))}
	timeouts := config.Timeouts{Privy: time.Second, RPC: time.Second}
	rpc := config.Solana{RPCURL: "http://127.0.0.1/rpc/"}
	for name, c := range map[string]struct {
		cfg config.Config
		ok  bool
	}{
		"no privy key":   {cfg: config.Config{Relayer: relayer, Timeouts: timeouts, Solana: rpc}},
		"no relayer key": {cfg: config.Config{Privy: privy, Timeouts: timeouts, Solana: rpc}},
		"both":           {cfg: config.Config{Privy: privy, Relayer: relayer, Timeouts: timeouts, Solana: rpc}, ok: true},
	} {
		m := treasury.New(module.Deps{Config: c.cfg, Clock: testkit.NewClock(time.Time{})})
		if err := m.BuildTransfers(); (err == nil) != c.ok {
			t.Errorf("%s: BuildTransfers = %v", name, err)
		}
		if m.Statuses() == nil {
			t.Errorf("%s: no signature status reader", name)
		}
	}
}
