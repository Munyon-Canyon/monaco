package trading_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (e *routeEnv) poller() *app.EVMTrades {
	return app.NewEVMTrades(e.pool, e.clk, e.exec(), app.DefaultSweepTiming())
}

func (e *routeEnv) tick(ctx context.Context, t *testing.T) (poller.Report, error) {
	t.Helper()
	return e.poller().Tick(actorContext(ctx))
}

func TestEVMTrades_resumesARoutedSwapOnceItIsOlderThanTheSweepAge(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	view := e.mustBuy(t, e.request(), domain.StatusCreated)
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}

	if report, err := e.tick(t.Context(), t); err != nil || report.Scanned != 0 ||
		e.poller().Interval() != app.SwapSweepInterval {
		t.Fatalf("young Tick = %+v, %v every %s, want nothing scanned", report, err, e.poller().Interval())
	}
	e.clk.Advance(app.SwapSweepAge + time.Second)
	if report, err := e.tick(t.Context(), t); err != nil || report.Scanned != 1 || report.Changed != 1 {
		t.Fatalf("Tick = %+v, %v, want the routed swap resumed", report, err)
	}
	if status, _, _, _ := e.row(t, view.ID.UUID()); status != string(domain.StatusConfirmed) {
		t.Fatalf("swap is %s, want confirmed", status)
	}
	if report, err := e.tick(t.Context(), t); err != nil || report.Scanned != 0 {
		t.Fatalf("Tick after the confirm = %+v, %v, want nothing scanned", report, err)
	}
	if len(e.privy.calls()) != 1 || e.evm.hashes() != 2 {
		t.Fatalf("transfers %v sends %v, want one bridge and one swap", e.privy.calls(), e.evm.keys())
	}
}

func TestEVMTrades_leavesUnroutedSwapsToTheSweeper(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	if _, err := e.layer().Run(actorContext(t.Context()), e.layerEnv.request(e.ids.NewV7()), nil); err != nil {
		t.Fatal(err)
	}
	e.clk.Advance(app.SwapSweepAge + time.Second)

	if report, err := e.tick(t.Context(), t); err != nil || report.Scanned != 0 {
		t.Fatalf("Tick = %+v, %v, want the Solana swap left alone", report, err)
	}
}

func TestEVMTrades_reportsAResumeErrorAndAListError(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.mustBuy(t, e.request(), domain.StatusCreated)
	e.clk.Advance(app.SwapSweepAge + time.Second)
	e.privy.actionErr = errs.New(errs.CodePrivyUnavailable, "privy.Action")

	if report, err := e.tick(t.Context(), t); errs.CodeOf(err) != errs.CodePrivyUnavailable || report.Changed != 0 {
		t.Fatalf("Tick = %+v, %v, want privy_unavailable", report, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := e.tick(ctx, t); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Tick on a cancelled context err = %v, want internal", err)
	}
}

func TestModule_resumesRoutedSwapsThroughTheMarketCatalogsListings(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.mustBuy(t, e.request(), domain.StatusCreated)
	e.clk.Advance(app.SwapSweepAge + time.Second)
	cfg := testkit.Config()
	cfg.Solana.RPCURL, cfg.Timeouts.RPC = "http://fakes/rpc/", time.Second
	m := trading.New(module.Deps{Config: cfg, Clock: e.clk, IDs: e.ids, Pool: e.pool, UoW: e.uow},
		trading.WithEnginePorts(e.ports()), trading.WithChain(e.venue, e.signer))

	pollers := m.Pollers()
	report, err := pollers[1].Tick(actorContext(t.Context()))
	if pollers[1].Name() != "trading.evm_trades" || report.Scanned != 1 ||
		errs.CodeOf(err) != errs.CodeAssetUntradable {
		t.Fatalf("%s Tick = %+v, %v, want the catalog refusing the unverified listing",
			pollers[1].Name(), report, err)
	}
}
