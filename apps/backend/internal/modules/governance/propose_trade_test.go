package governance_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type txGuard struct {
	t    *testing.T
	uow  *db.UnitOfWork
	open atomic.Bool
}

func (g *txGuard) Do(ctx context.Context, fn func(ctx context.Context, tx db.Tx) error) error {
	g.open.Store(true)
	defer g.open.Store(false)
	return g.uow.Do(ctx, fn)
}

func (g *txGuard) port(name string) {
	if g.open.Load() {
		g.t.Errorf("%s was called inside uow.Do", name)
	}
}

type guardedCabals struct {
	app.Cabals
	g *txGuard
}

type malformedRules struct{ app.Cabals }

func (malformedRules) Rules(context.Context, ids.CabalID) (cabal.Rules, error) {
	return cabal.Rules{Threshold: "bad"}, nil
}

func (c guardedCabals) IsMember(ctx context.Context, id ids.CabalID, user ids.UserID) (bool, error) {
	c.g.port("IsMember")
	return c.Cabals.IsMember(ctx, id, user)
}

func (c guardedCabals) Rules(ctx context.Context, id ids.CabalID) (cabal.Rules, error) {
	c.g.port("Rules")
	return c.Cabals.Rules(ctx, id)
}

func (c guardedCabals) TreasuryWallet(ctx context.Context, id ids.CabalID) (cabal.TreasuryWallet, error) {
	c.g.port("TreasuryWallet")
	return c.Cabals.TreasuryWallet(ctx, id)
}

func (c guardedCabals) VoterSet(ctx context.Context, id ids.CabalID) ([]ids.UserID, error) {
	c.g.port("VoterSet")
	return c.Cabals.VoterSet(ctx, id)
}

type guardedMarket struct {
	app.Assets
	market.Routes
	g *txGuard
}

func (m guardedMarket) AssetBySymbol(ctx context.Context, symbol string) (market.Asset, error) {
	m.g.port("AssetBySymbol")
	return m.Assets.AssetBySymbol(ctx, symbol)
}

func (m guardedMarket) CheckRoute(
	ctx context.Context, id market.AssetID, side market.Side, amount money.BaseUnits,
) (market.RouteCheck, error) {
	m.g.port("CheckRoute")
	return m.Routes.CheckRoute(ctx, id, side, amount)
}

type guardedTreasury struct {
	app.Treasury
	g *txGuard
}

func (r guardedTreasury) PotValue(ctx context.Context, id ids.CabalID) (money.Micros, error) {
	r.g.port("PotValue")
	return r.Treasury.PotValue(ctx, id)
}

func (r guardedTreasury) Positions(ctx context.Context, id ids.CabalID) ([]treasury.Position, error) {
	r.g.port("Positions")
	return r.Treasury.Positions(ctx, id)
}

type guardedBalances struct {
	app.Balances
	g *txGuard
}

func (b guardedBalances) TokenBalance(
	ctx context.Context, owner chain.SolanaAddress, mint chain.Mint,
) (money.BaseUnits, error) {
	b.g.port("TokenBalance")
	return b.Balances.TokenBalance(ctx, owner, mint)
}

type sameID struct{ u uuid.UUID }

func (s sameID) NewV7() uuid.UUID { return s.u }

type proposeHarness struct {
	w     *tradeWorld
	guard *txGuard
	d     proposalDB
}

func newProposeHarness(t *testing.T) proposeHarness {
	t.Helper()
	d := newProposalDB(t)
	return proposeHarness{
		w: newTradeWorld(t), d: d, guard: &txGuard{t: t, uow: db.New(d.pool, d.ids, clock.Real{})},
	}
}

func (h proposeHarness) handler(g ids.Generator) *app.ProposeTradeHandler {
	return h.handlerWithHints(g, app.NoHints{})
}

func (h proposeHarness) handlerWithHints(g ids.Generator, hints app.Hints) *app.ProposeTradeHandler {
	p := h.w.ports()
	m := guardedMarket{Assets: p.Assets, Routes: p.Routes, g: h.guard}
	return app.NewProposeTradeHandler(h.guard, g, clock.Real{}, app.TradePorts{
		Cabals: guardedCabals{p.Cabals, h.guard}, Assets: m, Routes: m, Treasury: guardedTreasury{p.Treasury, h.guard},
		Balances: guardedBalances{p.Balances, h.guard},
	}, hints)
}

func (h proposeHarness) propose(g ids.Generator, trade domain.Trade) (ids.ProposalID, error) {
	return h.proposeWithHints(g, trade, app.NoHints{})
}

func (h proposeHarness) proposeWithHints(g ids.Generator, trade domain.Trade, hints app.Hints) (ids.ProposalID, error) {
	ctx := observability.WithActor(h.guard.t.Context(), "user:"+h.w.members[0].String())
	return h.handlerWithHints(g, hints).Handle(ctx, app.ProposeTrade{
		CabalID: h.w.cabal, ProposerID: h.w.members[0], Trade: trade,
	})
}

func buyAAPLFor(micros uint64) domain.Trade {
	return domain.Trade{Kind: domain.KindBuy, Symbol: "AAPLx", USDCMicros: money.MicrosFromUint64(micros)}
}

func sellAAPL(units uint64) domain.Trade {
	return domain.Trade{Kind: domain.KindSell, Symbol: "AAPLx", TokenAmount: units}
}

func TestProposeTrade_callsNoPortInsideTheTransaction(t *testing.T) {
	t.Parallel()
	h := newProposeHarness(t)
	for _, trade := range []domain.Trade{
		sellAAPL(heldUnits),
		buyAAPLFor(potMicros),
	} {
		id, err := h.propose(h.d.ids, trade)
		if err != nil {
			t.Fatalf("propose %s = %v", trade.Kind, err)
		}
		voters, err := h.d.q.ProposalVoters(t.Context(), id.UUID())
		if err != nil || len(voters) != len(h.w.members) {
			t.Fatalf("%s voters = %d, %v, want %d", trade.Kind, len(voters), err, len(h.w.members))
		}
	}
}

func TestProposeTrade_sellAboveOnChainBalanceIsRefused(t *testing.T) {
	t.Parallel()
	h := newProposeHarness(t)
	aapl := marketfake.AAPLx()
	for name, tc := range map[string]struct {
		ledger, onChain uint64
		sell            uint64
		want            errs.Code
	}{
		"ledger short, on-chain enough": {heldUnits - 1, heldUnits, heldUnits, errs.CodeInsufficientFunds},
		"on-chain short, ledger enough": {heldUnits, heldUnits - 1, heldUnits, errs.CodeInsufficientFunds},
		"both enough":                   {heldUnits, heldUnits, heldUnits, ""},
	} {
		h.w = newTradeWorld(t)
		h.w.treasury.SetPositions(h.w.cabal, []treasury.Position{
			{Mint: aapl.Mint.Address(), Units: money.NewBaseUnits(tc.ledger, aapl.Decimals)},
		})
		h.w.holdOnChain(tc.onChain)
		_, err := h.propose(h.d.ids, sellAAPL(tc.sell))
		if (tc.want == "") != (err == nil) || err != nil && errs.CodeOf(err) != tc.want {
			t.Errorf("%s: propose sell err = %v, want %q", name, err, tc.want)
		}
	}
}

type createdHints struct {
	guard   *txGuard
	created []string
}

func (h *createdHints) ProposalCreated(_ context.Context, cabal ids.CabalID, proposal ids.ProposalID) {
	h.guard.port("ProposalCreated")
	h.created = append(h.created, cabal.String()+":"+proposal.String())
}

func (*createdHints) ProposalUpdated(context.Context, ids.CabalID, ids.ProposalID) {}

func TestProposeTrade_publishesCreatedAfterCommit(t *testing.T) {
	t.Parallel()
	h := newProposeHarness(t)
	hints := &createdHints{guard: h.guard}
	id, err := h.proposeWithHints(h.d.ids, buyAAPLFor(potMicros), hints)
	if err != nil {
		t.Fatal(err)
	}
	want := h.w.cabal.String() + ":" + id.String()
	if len(hints.created) != 1 || hints.created[0] != want {
		t.Fatalf("created hints = %q, want %q", hints.created, want)
	}
}

func TestProposeTrade_refusesWhatThePortsRefuse(t *testing.T) {
	t.Parallel()
	h := newProposeHarness(t)
	failed := errs.New(errs.CodeUpstreamUnavailable, "test")
	for name, tc := range map[string]struct {
		arrange func(w *tradeWorld)
		trade   domain.Trade
		want    errs.Code
	}{
		"membership unknown": {
			func(w *tradeWorld) { w.cabals.current.Load().Fail("IsMember", failed) },
			sellAAPL(1), errs.CodeUpstreamUnavailable,
		},
		"rules unknown": {
			func(w *tradeWorld) { w.cabals.current.Load().Fail("Rules", failed) },
			sellAAPL(1), errs.CodeUpstreamUnavailable,
		},
		"voters unknown": {
			func(w *tradeWorld) { w.cabals.current.Load().Fail("VoterSet", failed) },
			sellAAPL(1), errs.CodeUpstreamUnavailable,
		},
		"proposer left": {func(w *tradeWorld) {
			w.rows = w.rows[1:]
			w.join(w.members[1], true)
		}, sellAAPL(1), errs.CodeNotCabalMember},
		"nobody can vote": {func(w *tradeWorld) {
			w.rows = w.rows[:0]
			w.join(w.members[0], false)
		}, sellAAPL(1), errs.CodeInvalidInput},
		"positions unknown": {
			func(w *tradeWorld) { w.treasury.Fail("Positions", failed) },
			sellAAPL(1), errs.CodeUpstreamUnavailable,
		},
		"position in other units": {func(w *tradeWorld) {
			w.treasury.SetPositions(w.cabal, []treasury.Position{
				{Mint: marketfake.AAPLx().Mint.Address(), Units: money.NewBaseUnits(heldUnits, 6)},
			})
		}, sellAAPL(1), errs.CodeInternal},
		"wallet unknown": {
			func(w *tradeWorld) { w.cabals.current.Load().Fail("TreasuryWallet", failed) },
			sellAAPL(1), errs.CodeUpstreamUnavailable,
		},
		"balance unknown": {
			func(w *tradeWorld) { w.chain.Fail("TokenBalance", failed) },
			sellAAPL(1), errs.CodeUpstreamUnavailable,
		},
		"asset unknown": {
			func(*tradeWorld) {},
			domain.Trade{Kind: domain.KindSell, Symbol: "NOPEx", TokenAmount: 1},
			errs.CodeAssetNotFound,
		},
		"pot unknown": {
			func(w *tradeWorld) { w.treasury.Fail("PotValue", failed) }, buyAAPLFor(1),
			errs.CodeUpstreamUnavailable,
		},
		"buy over the pot":  {func(*tradeWorld) {}, buyAAPLFor(potMicros + 1), errs.CodePotExceeded},
		"sell over holding": {func(*tradeWorld) {}, sellAAPL(heldUnits + 1), errs.CodeInsufficientFunds},
		"no route": {func(*tradeWorld) {}, domain.Trade{
			Kind: domain.KindBuy, Symbol: "TSLAx", USDCMicros: money.MicrosFromUint64(1),
		}, errs.CodeNoRoute},
		"quote of nothing": {func(w *tradeWorld) {
			w.routes.Ok(marketfake.AAPLx().ID, market.RouteCheck{OutAmount: money.NewBaseUnits(0, 6)})
		}, sellAAPL(1), errs.CodeInvalidInput},
	} {
		h.w = newTradeWorld(t)
		tc.arrange(h.w)
		hints := &createdHints{guard: h.guard}
		if _, err := h.proposeWithHints(h.d.ids, tc.trade, hints); errs.CodeOf(err) != tc.want {
			t.Errorf("%s: propose err = %v, want %s", name, err, tc.want)
		}
		if len(hints.created) != 0 {
			t.Errorf("%s: created hints = %q, want none", name, hints.created)
		}
	}
}

func TestProposeTrade_aFailedInsertWritesNothing(t *testing.T) {
	t.Parallel()
	h := newProposeHarness(t)
	same := sameID{h.d.ids.NewV7()}
	if _, err := h.propose(same, sellAAPL(1)); err != nil {
		t.Fatal(err)
	}
	if _, err := h.propose(same, sellAAPL(2)); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("second insert with the same id err = %v, want internal", err)
	}
}

func TestProposeTrade_rejectsMalformedThresholdAfterOpening(t *testing.T) {
	t.Parallel()
	h := newProposeHarness(t)
	p := h.w.ports()
	handler := app.NewProposeTradeHandler(h.guard, h.d.ids, clock.Real{}, app.TradePorts{
		Cabals: malformedRules{p.Cabals}, Assets: p.Assets, Routes: p.Routes, Treasury: p.Treasury,
	}, app.NoHints{})
	ctx := observability.WithActor(t.Context(), "user:"+h.w.members[0].String())
	_, err := handler.Handle(ctx, app.ProposeTrade{
		CabalID: h.w.cabal, ProposerID: h.w.members[0], Trade: buyAAPLFor(potMicros),
	})
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("Handle err = %v, want decode_failed", err)
	}
}

func TestProposeTrade_nonVoterIsRefused(t *testing.T) {
	t.Parallel()
	h := newProposeHarness(t)
	h.w.rows = h.w.rows[:0]
	h.w.join(h.w.members[0], false)
	h.w.join(h.w.members[1], true)
	hints := &createdHints{guard: h.guard}
	if _, err := h.proposeWithHints(h.d.ids, buyAAPLFor(1), hints); errs.CodeOf(err) != errs.CodeNotAVoter {
		t.Fatalf("propose err = %v, want %s", err, errs.CodeNotAVoter)
	}
	if len(hints.created) != 0 {
		t.Errorf("created hints = %q, want none", hints.created)
	}
}
