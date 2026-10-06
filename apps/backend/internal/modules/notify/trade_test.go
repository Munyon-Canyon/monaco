package notify_test

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const (
	tradeActor   = "system:trading.engine"
	proposalKind = "proposal"
	cashOutKind  = "cashout"
	fiftyUSDC    = 50_000_000
)

type tradeLeg struct{ source, action, symbol string }

type tradeRun struct {
	delivery bus.Delivery
	swap     uuid.UUID
	err      error
}

type tradePorts struct {
	cabals app.Cabals
	assets app.Assets
}

type (
	tradeEventFunc  func(t *testing.T, r *pushRig, cabalID ids.CabalID, leg tradeLeg) events.Event
	tradeHandleFunc func(t *testing.T, r *pushRig, ports tradePorts, d bus.Delivery, e events.Event) error
)

type tradeKind struct {
	name, kind, action, title, body, unnamedBody string

	event  tradeEventFunc
	handle tradeHandleFunc
}

func (k tradeKind) run(t *testing.T, r *pushRig, ports tradePorts, cabalID ids.CabalID, leg tradeLeg) tradeRun {
	t.Helper()
	e := k.event(t, r, cabalID, leg)
	d := r.emit(t, tradeActor, e)
	return tradeRun{delivery: d, swap: e.AggregateID(), err: k.handle(t, r, ports, d, e)}
}

func filledKind(action, body, unnamedBody string) tradeKind {
	return tradeKind{
		name: action, kind: "trade_filled", action: action, title: "Trade filled", body: body,
		unnamedBody: unnamedBody,
		event: func(t *testing.T, r *pushRig, cabalID ids.CabalID, leg tradeLeg) events.Event {
			t.Helper()
			e := goldenEvent(t, events.TypeTradeConfirmed).(events.TradeConfirmed)
			e.SwapID, e.CabalID, e.Source.Kind = r.ids.NewV7(), cabalID.UUID(), leg.source
			e.Action, e.Symbol, e.USDCMicros = leg.action, leg.symbol, money.MicrosFromUint64(fiftyUSDC)
			return e
		},
		handle: func(t *testing.T, r *pushRig, ports tradePorts, d bus.Delivery, e events.Event) error {
			t.Helper()
			kind := app.TradeFilled{Cabals: ports.cabals, Assets: ports.assets}
			return handleKinds(t, r, r.sender, d, e.(events.TradeConfirmed), kind)
		},
	}
}

func failedKind(action, body, unnamedBody string) tradeKind {
	return tradeKind{
		name: action, kind: "trade_failed", action: action, title: "Trade didn't go through", body: body,
		unnamedBody: unnamedBody,
		event: func(t *testing.T, r *pushRig, cabalID ids.CabalID, leg tradeLeg) events.Event {
			t.Helper()
			e := goldenEvent(t, events.TypeTradeFailed).(events.TradeFailed)
			e.SwapID, e.CabalID, e.Source.Kind = r.ids.NewV7(), cabalID.UUID(), leg.source
			e.Action, e.Symbol = leg.action, leg.symbol
			return e
		},
		handle: func(t *testing.T, r *pushRig, ports tradePorts, d bus.Delivery, e events.Event) error {
			t.Helper()
			kind := app.TradeFailed{Cabals: ports.cabals, Assets: ports.assets}
			return handleKinds(t, r, r.sender, d, e.(events.TradeFailed), kind)
		},
	}
}

func tradeKinds() []tradeKind {
	return []tradeKind{
		filledKind("buy", "Your cabal bought $50.00 of Apple", "Your cabal bought $50.00 of a stock"),
		filledKind("sell", "Your cabal sold $50.00 of Apple", "Your cabal sold $50.00 of a stock"),
		failedKind("buy", "Your cabal's buy of Apple failed. No money moved.",
			"Your cabal's buy of a stock failed. No money moved."),
		failedKind("sell", "Your cabal's sell of Apple failed. No money moved.",
			"Your cabal's sell of a stock failed. No money moved."),
	}
}

func tradeKindsWhere(keep func(tradeKind) bool) []tradeKind {
	return slices.DeleteFunc(tradeKinds(), func(k tradeKind) bool { return !keep(k) })
}

func fixtureCatalog() *marketfake.CatalogFake { return marketfake.NewCatalog(marketfake.Fixtures()...) }

func (r *pushRig) wantNothingPushed(t *testing.T, run tradeRun) {
	t.Helper()
	if sent := r.sender.Sent(); len(sent) != 0 {
		t.Fatalf("sent %+v, want nothing", sent)
	}
	r.wantStates(t, run.delivery, map[ids.UserID]string{})
}

func wantTradePushedToEveryMember(t *testing.T, k tradeKind) {
	t.Helper()
	r := newPushRig(t)
	w := r.cabalOf(t, 3)

	run := k.run(t, r, tradePorts{w.cabals, fixtureCatalog()}, w.id, tradeLeg{proposalKind, k.action, "AAPLx"})

	wantVerdict(t, run.err, "", errs.VerdictAck)
	want := make([]apns.Push, 0, len(w.members))
	states := map[ids.UserID]string{}
	for i, member := range w.members {
		states[member] = "delivered"
		want = append(want, apns.Push{
			UserID: member, Token: token(byte('a' + i)), Environment: apns.Sandbox,
			CollapseID: "trade-" + run.swap.String(), Title: k.title, Body: k.body,
			Data: map[string]string{"kind": k.kind, "cabal_id": w.id.String(), "txn_id": run.swap.String()},
		})
	}
	sent := r.sender.Sent()
	slices.SortFunc(sent, func(a, b apns.Push) int { return strings.Compare(a.Token, b.Token) })
	if !reflect.DeepEqual(sent, want) {
		t.Fatalf("sent %+v, want one push per member and none to the outsider: %+v", sent, want)
	}
	r.wantStates(t, run.delivery, states)
	r.wantCount(t, "rows in one broadcast of three", 3, `SELECT count(*) FROM notifications n
		JOIN notification_broadcasts b ON b.id = n.broadcast_id
		WHERE b.source_event_id = $1 AND b.recipient_count = 3 AND b.kind = $2`, run.delivery.EventID.UUID(), k.kind)
	r.wantSentEvents(t, run.delivery, 3)
	r.wantRecorded(t, run.delivery, 1)
}

func TestNotify_TradeFilled_AllMembers(t *testing.T) {
	t.Parallel()
	for _, k := range tradeKindsWhere(func(k tradeKind) bool { return k.kind == "trade_filled" }) {
		t.Run(k.name, func(t *testing.T) {
			t.Parallel()
			wantTradePushedToEveryMember(t, k)
		})
	}
}

func TestNotify_TradeFailed_AllMembers(t *testing.T) {
	t.Parallel()
	for _, k := range tradeKindsWhere(func(k tradeKind) bool { return k.kind == "trade_failed" }) {
		t.Run(k.name, func(t *testing.T) {
			t.Parallel()
			wantTradePushedToEveryMember(t, k)
		})
	}
}

func TestNotify_Trade_ACashOutNotifiesNobodyAndReadsNoPort(t *testing.T) {
	t.Parallel()
	for _, k := range tradeKinds() {
		t.Run(k.kind+"/"+k.name, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			w := r.cabalOf(t, 2)
			down := errs.New(errs.CodeDBUnavailable, "test.port")
			w.cabals.Fail("Members", down)
			w.cabals.Fail("Cabal", down)
			assets := fixtureCatalog()
			assets.Fail("AssetBySymbol", down)

			run := k.run(t, r, tradePorts{w.cabals, assets}, w.id, tradeLeg{cashOutKind, k.action, "AAPLx"})

			wantVerdict(t, run.err, "", errs.VerdictAck)
			r.wantNothingPushed(t, run)
			r.wantRecorded(t, run.delivery, 1)
		})
	}
}

func TestNotify_Trade_AnUnknownSymbolReadsAsAStock(t *testing.T) {
	t.Parallel()
	for _, k := range tradeKinds() {
		t.Run(k.kind+"/"+k.name, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			w := r.cabalOf(t, 2)

			run := k.run(t, r, tradePorts{w.cabals, fixtureCatalog()}, w.id, tradeLeg{proposalKind, k.action, "ZZZZx"})

			wantVerdict(t, run.err, "", errs.VerdictAck)
			sent := r.sender.Sent()
			if len(sent) != 2 {
				t.Fatalf("sent %d pushes, want one per member of 2", len(sent))
			}
			for _, p := range sent {
				if p.Title != k.title || p.Body != k.unnamedBody {
					t.Errorf("push %q / %q, want %q / %q", p.Title, p.Body, k.title, k.unnamedBody)
				}
			}
			r.wantRecorded(t, run.delivery, 1)
		})
	}
}

func TestNotify_Trade_PortFailuresReturnAsIsAndWriteNothing(t *testing.T) {
	t.Parallel()
	for _, k := range tradeKindsWhere(func(k tradeKind) bool { return k.action == "buy" }) {
		for _, op := range []string{"Members", "AssetBySymbol"} {
			t.Run(k.kind+"/"+op, func(t *testing.T) {
				t.Parallel()
				r := newPushRig(t)
				w := r.cabalOf(t, 2)
				assets := fixtureCatalog()
				down := errs.New(errs.CodeDBUnavailable, "test."+op)
				w.cabals.Fail(op, down)
				assets.Fail(op, down)

				run := k.run(t, r, tradePorts{w.cabals, assets}, w.id, tradeLeg{proposalKind, k.action, "AAPLx"})

				if !errors.Is(run.err, down) {
					t.Fatalf("Handle = %v, want the port error itself, %v", run.err, down)
				}
				wantVerdict(t, run.err, errs.CodeDBUnavailable, errs.VerdictNak)
				r.wantNothingPushed(t, run)
				r.wantRecorded(t, run.delivery, 0)
			})
		}
	}
}

func TestNotify_Trade_RefusesAnActionItCannotPhraseAndWritesNothing(t *testing.T) {
	t.Parallel()
	for _, k := range tradeKindsWhere(func(k tradeKind) bool { return k.action == "buy" }) {
		t.Run(k.kind, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			w := r.cabalOf(t, 2)

			run := k.run(t, r, tradePorts{w.cabals, fixtureCatalog()}, w.id, tradeLeg{proposalKind, "swap", "AAPLx"})

			wantVerdict(t, run.err, errs.CodeInvalidInput, errs.VerdictTerm)
			r.wantNothingPushed(t, run)
			r.wantRecorded(t, run.delivery, 0)
		})
	}
}

func TestNotify_TradeRecipients_AreTheMembersInPortOrderForAProposalOnly(t *testing.T) {
	t.Parallel()
	g := testkit.NewIDs(1)
	cabalID := ids.CabalIDFrom(g.NewV7())
	first, second, third := ids.NewUserID(g), ids.NewUserID(g), ids.NewUserID(g)
	joined := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	member := func(user ids.UserID, after time.Duration) fakes.CabalMember {
		return fakes.CabalMember{CabalID: cabalID, Member: cabal.MemberView{UserID: user, JoinedAt: joined.Add(after)}}
	}
	cabals := fakes.NewCabal(
		[]fakes.CabalSeed{{View: cabal.View{ID: cabalID, Name: cabalName}}},
		[]fakes.CabalMember{member(third, 0), member(first, 2*time.Minute), member(second, time.Minute)},
	)
	want := []ids.UserID{third, second, first}
	confirmed := goldenEvent(t, events.TypeTradeConfirmed).(events.TradeConfirmed)
	failed := goldenEvent(t, events.TypeTradeFailed).(events.TradeFailed)
	confirmed.CabalID, failed.CabalID = cabalID.UUID(), cabalID.UUID()

	for source, expected := range map[string][]ids.UserID{proposalKind: want, cashOutKind: nil} {
		confirmed.Source.Kind, failed.Source.Kind = source, source
		filledTo, filledErr := app.TradeFilled{Cabals: cabals}.Recipients(t.Context(), confirmed)
		failedTo, failedErr := app.TradeFailed{Cabals: cabals}.Recipients(t.Context(), failed)

		if filledErr != nil || failedErr != nil {
			t.Fatalf("%s source: Recipients = %v and %v, want no error", source, filledErr, failedErr)
		}
		if !slices.Equal(filledTo, expected) || !slices.Equal(failedTo, expected) {
			t.Errorf("%s source: filled to %v and failed to %v, want %v for both", source, filledTo, failedTo, expected)
		}
	}
}

func insertRealAsset(t *testing.T, r *pushRig, a market.Asset) {
	t.Helper()
	r.exec(t, `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name, issuer_tradable,
		company_key, first_seen_at, updated_at, chain_checked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, true, $8, $9, $9, $9)`,
		a.ID.UUID(), a.Symbol, a.Mint.String(), int16(a.Decimals), string(a.Issuer), string(a.Kind), a.DisplayName,
		a.CompanyKey, r.clock.Now())
}

func (r *pushRig) wantModulePushedTrade(t *testing.T, d bus.Delivery, k tradeKind, body string, members []ids.UserID) {
	t.Helper()
	states := map[ids.UserID]string{}
	for _, member := range members {
		states[member] = "delivered"
	}
	r.wantStates(t, d, states)
	sent := r.sender.Sent()
	if len(sent) != len(members) {
		t.Fatalf("sent %d pushes, want one per member of %d", len(sent), len(members))
	}
	for _, p := range sent {
		if p.Title != k.title || p.Body != body {
			t.Errorf("push %q / %q, want %q / %q", p.Title, p.Body, k.title, body)
		}
	}
	r.wantRecorded(t, d, 1)
}

func TestNotify_Trade_ReachesTheMembersOfARealCabalAndNamesItsRealAssetByDefault(t *testing.T) {
	t.Parallel()
	for _, k := range tradeKinds() {
		t.Run(k.kind+"/"+k.name, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			c := testkit.NewCabal(t, r.pool, testkit.WithMembers(3), testkit.WithName(cabalName))
			members := make([]ids.UserID, len(c.Members))
			for i, member := range c.Members {
				members[i] = member.ID
				r.device(t, member.ID, token(byte('a'+i)))
			}
			r.device(t, r.user(t, "active"), token('z'))
			apple := marketfake.AAPLx()
			insertRealAsset(t, r, apple)
			deps := module.Deps{Pool: r.pool, UoW: r.uow, IDs: r.ids, Clock: r.clock}
			e := k.event(t, r, c.ID, tradeLeg{proposalKind, k.action, apple.Symbol})

			d := r.dispatchTo(t, notify.New(deps, notify.WithSender(r.sender)), tradeActor, e)

			r.wantModulePushedTrade(t, d, k, k.body, members)
		})
	}
}

func TestNotify_Trade_ReachesTheMembersAndNamesTheAssetOfThePortsItIsGiven(t *testing.T) {
	t.Parallel()
	for _, k := range tradeKinds() {
		t.Run(k.kind+"/"+k.name, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			w := r.cabalOf(t, 2)
			pear := marketfake.AAPLx()
			pear.DisplayName = "Pear"
			deps := module.Deps{Pool: r.pool, UoW: r.uow, IDs: r.ids, Clock: r.clock}
			m := notify.New(deps, notify.WithSender(r.sender), notify.WithCabals(w.cabals),
				notify.WithAssets(marketfake.NewCatalog(pear)))
			e := k.event(t, r, w.id, tradeLeg{proposalKind, k.action, pear.Symbol})

			d := r.dispatchTo(t, m, tradeActor, e)

			r.wantModulePushedTrade(t, d, k, strings.Replace(k.body, "Apple", "Pear", 1), w.members)
		})
	}
}
