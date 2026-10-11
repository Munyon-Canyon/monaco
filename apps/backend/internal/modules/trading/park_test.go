package trading_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	governance "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	mcfakes "github.com/monaco/monaco/apps/backend/internal/testkit/fakes/multichain"
)

func (e *routeEnv) parked(t *testing.T, swap uuid.UUID) (string, string) {
	t.Helper()
	var status, code string
	if err := e.pool.QueryRow(t.Context(), `SELECT status, coalesce(parked_code, '') FROM swaps WHERE id = $1`, swap).
		Scan(&status, &code); err != nil {
		t.Fatal(err)
	}
	return status, code
}

func (e *routeEnv) bridgedThenStuck(t *testing.T) uuid.UUID {
	t.Helper()
	const code = errs.CodeUpstreamUnavailable
	e.quotes = mcfakes.New().Quoter
	e.quotes.Quotes.Push(mcfakes.Reply[multichain.Quote]{Code: code})
	e.privy.script(multichain.ActionSucceeded)
	cmd := e.engineEnv.buy()
	cmd.Route = e.route
	err := e.tradeHandler().Handle(actorContext(t.Context()), e.delivery(t, cmd), cmd, nil)
	if errs.CodeOf(err) != code {
		t.Fatalf("Handle err = %v, want %s", err, code)
	}
	e.clk.Advance(app.SwapSweepAge + time.Second)
	return e.swapsOf(t, cmd.ProposalID)[0]
}

func TestEVMTrades_aTerminalResumeErrorParksTheSwapAndAlertsOnce(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.bridgedThenStuck(t)
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
	e.listings = listingsFake{}

	if report, err := e.tick(ctx, t); errs.CodeOf(err) != errs.CodeAssetNotFound || report.Scanned != 1 {
		t.Fatalf("first Tick = %+v, %v, want the missing listing reported", report, err)
	}
	for range 2 {
		if report, err := e.tick(ctx, t); err != nil || report.Scanned != 0 {
			t.Fatalf("later Tick = %+v, %v, want the parked swap skipped", report, err)
		}
	}

	status, code := e.parked(t, swap)
	if status != string(domain.StatusCreated) || code != string(errs.CodeAssetNotFound) {
		t.Fatalf("swap %s parked %q, want created and parked as asset_not_found", status, code)
	}
	alerts := logs.Lines(observability.TradingRoutedSwapParked.Name, "level", "swap_id", "cabal_id", "code", "step")
	want := "ERROR " + swap.String() + " " + e.cabal.String() + " asset_not_found bridge_out"
	if len(alerts) != 1 || alerts[0] != want {
		t.Fatalf("park alerts = %q, want one %q", alerts, want)
	}
	if n := len(e.privy.calls()); n != 1 {
		t.Fatalf("%d transfers, want only the bridge out", n)
	}
}

func TestEVMTrades_aRetryableResumeErrorKeepsRetryingUnparked(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.bridgedThenStuck(t)

	for range 3 {
		e.quotes.Quotes.Push(mcfakes.Reply[multichain.Quote]{Code: errs.CodeUpstreamUnavailable})
		report, err := e.tick(t.Context(), t)
		if errs.CodeOf(err) != errs.CodeUpstreamUnavailable || report.Scanned != 1 {
			t.Fatalf("Tick = %+v, %v, want the outage retried", report, err)
		}
	}
	if status, code := e.parked(t, swap); status != string(domain.StatusCreated) || code != "" {
		t.Fatalf("swap %s parked %q, want created and unparked", status, code)
	}
}

func TestEVMTrades_bridgesBackAParkedSwapsCashOnceItsProposalIsDead(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.bridgedThenStuck(t)
	listed := e.listings
	e.listings = listingsFake{}
	if _, err := e.tick(t.Context(), t); errs.CodeOf(err) != errs.CodeAssetNotFound {
		t.Fatalf("Tick err = %v, want the swap parked", err)
	}
	e.listings = listed
	e.clk.Advance(app.SwapSweepAge + time.Second)
	if _, err := e.tick(t.Context(), t); err != nil || len(e.privy.calls()) != 1 {
		t.Fatalf("Tick = %v after %d transfers, want the cash kept while the proposal can retry", err,
			len(e.privy.calls()))
	}

	e.proposals.set(governance.Status("cancelled"))
	if report, err := e.tick(t.Context(), t); err != nil || report.Changed != 1 {
		t.Fatalf("Tick = %+v, %v, want the parked cash bridged back", report, err)
	}

	calls := e.privy.calls()
	if len(calls) != 2 || calls[1].key != "trade:"+swap.String()+":bridge_back" {
		t.Fatalf("transfers = %+v, want the bridge out then the bridge back", calls)
	}
}

type finishingResumer struct {
	app.Resumer
	t    *testing.T
	e    *routeEnv
	stmt string
}

func (f finishingResumer) Resume(ctx context.Context, id ids.SwapID) error {
	args := []any{id.UUID()}
	if !strings.Contains(f.stmt, "$1") {
		args = nil
	}
	if _, err := f.e.pool.Exec(ctx, f.stmt, args...); err != nil {
		f.t.Fatal(err)
	}
	return errs.New(errs.CodeAssetUntradable, "test.Resume")
}

func TestEVMTrades_parksOnlyASwapStillUnfinishedAndReportsAParkFailure(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		stmt     string
		parkFail bool
	}{
		"finished meanwhile": {stmt: `UPDATE swaps SET status = 'confirmed', confirmed_at = now() WHERE id = $1`},
		"park unwritable":    {stmt: `ALTER TABLE swaps RENAME COLUMN parked_code TO parked_reason`, parkFail: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			swap := e.bridgedThenStuck(t)
			logs := &testkit.Logs{}
			ctx := observability.WithLogger(actorContext(t.Context()),
				observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
			p := app.NewEVMTrades(e.uow, e.pool, e.clk, finishingResumer{t: t, e: e, stmt: c.stmt}, e.proposals,
				app.DefaultSweepTiming())

			_, err := p.Tick(ctx)

			parkFailed := strings.Contains(fmt.Sprint(err), "parked")
			if errs.CodeOf(err) != errs.CodeAssetUntradable || parkFailed != c.parkFail {
				t.Fatalf("Tick err = %v, want asset_untradable with a park failure %v", err, c.parkFail)
			}
			if n := len(logs.Lines(observability.TradingRoutedSwapParked.Name)); n != 0 {
				t.Fatalf("%d park alerts for %s, want none", n, swap)
			}
		})
	}
}
