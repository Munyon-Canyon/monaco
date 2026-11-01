package trading_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

type calendarFake struct {
	state market.SessionState
	err   error
	asked []market.AssetID
}

func (c *calendarFake) Session(_ context.Context, id market.AssetID, _ time.Time) (market.SessionInfo, error) {
	c.asked = append(c.asked, id)
	return market.SessionInfo{State: c.state}, c.err
}

func (e *routeEnv) handlerWith(calendar market.Calendar, network chain.Network) *app.ExecuteTradeHandler {
	listing := e.listing
	listing.Network = network
	e.listings.listings[listing.ID] = listing
	ports := e.ports()
	ports.Calendar = calendar
	return app.NewExecuteTradeHandler(app.ExecuteTradeDeps{
		Layer: e.layer(), Routes: e.exec(), Listings: e.listings, UoW: e.uow, Reads: e.reads, Venue: e.venue,
		Ports: ports, USDC: usdcToken(), EVM: true,
	})
}

func TestExecuteTrade_aRobinhoodBuyIsRefusedWhileTheMarketIsClosed(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	calendar := &calendarFake{state: market.SessionClosed}
	cmd := e.engineEnv.buy()
	cmd.Route = e.route
	d := e.delivery(t, cmd)

	err := e.handlerWith(calendar, chain.NetworkRobinhood).Handle(actorContext(t.Context()), d, cmd, nil)
	if err != nil {
		t.Fatal(err)
	}
	if code := e.deliveryCode(t, d); code != string(errs.CodeAssetPaused) || len(e.privy.calls()) != 0 {
		t.Fatalf("delivery %s after %d transfers, want asset_paused and no bridge", code, len(e.privy.calls()))
	}
	if n := len(e.swapsOf(t, cmd.ProposalID)); n != 0 || len(calendar.asked) != 1 {
		t.Fatalf("%d swaps, %d calendar reads, want no swap after one read", n, len(calendar.asked))
	}
}

func TestExecuteTrade_aCalendarFailureStopsARobinhoodBuyBeforeTheBridge(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	calendar := &calendarFake{err: errs.New(errs.CodeCalendarExpired, "test.calendar")}
	cmd := e.engineEnv.buy()
	cmd.Route = e.route
	d := e.delivery(t, cmd)

	err := e.handlerWith(calendar, chain.NetworkRobinhood).Handle(actorContext(t.Context()), d, cmd, nil)
	if err != nil {
		t.Fatal(err)
	}
	if code := e.deliveryCode(t, d); code != string(errs.CodeTradeNotStarted) || len(e.privy.calls()) != 0 {
		t.Fatalf("delivery %s after %d transfers, want trade_not_started and no bridge", code, len(e.privy.calls()))
	}
}

func TestExecuteTrade_theMarketHoursGateOnlyCoversRobinhood(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}
	e.quote(usdcToken(), aaplxToken(), 0, false)
	calendar := &calendarFake{state: market.SessionClosed}
	cmd := e.engineEnv.buy()
	cmd.Route = e.route
	d := e.delivery(t, cmd)

	err := e.handlerWith(calendar, chain.NetworkBase).Handle(actorContext(t.Context()), d, cmd, nil)
	if err != nil {
		t.Fatal(err)
	}
	if code := e.deliveryCode(t, d); code != "ok" || len(calendar.asked) != 0 {
		t.Fatalf("delivery %s after %d calendar reads, want a Base buy that never asks the calendar", code,
			len(calendar.asked))
	}
}

func TestExecuteTrade_aRobinhoodBuyPassesTheGateWhileTheMarketIsOpen(t *testing.T) {
	t.Parallel()
	for _, state := range []market.SessionState{"open", "pre_market", "after_hours"} {
		t.Run(string(state), func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			calendar := &calendarFake{state: state}
			cmd := e.engineEnv.buy()
			cmd.Route = e.route
			d := e.delivery(t, cmd)

			_ = e.handlerWith(calendar, chain.NetworkRobinhood).Handle(actorContext(t.Context()), d, cmd, nil)

			if len(calendar.asked) != 1 || len(e.swapsOf(t, cmd.ProposalID)) != 1 {
				t.Fatalf("%d calendar reads, %d swaps, want one read and a swap started", len(calendar.asked),
					len(e.swapsOf(t, cmd.ProposalID)))
			}
		})
	}
}
