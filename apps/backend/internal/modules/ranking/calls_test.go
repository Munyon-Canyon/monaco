package ranking_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	callsMint = "9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin"
	callsUSDC = "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
)

type callsMarket struct {
	asset  market.Asset
	price  market.Price
	priced bool
}

func newCallsMarket(t *testing.T, id uuid.UUID, price uint64, observed time.Time) callsMarket {
	t.Helper()
	mint, err := market.ParseMint(callsMint)
	if err != nil {
		t.Fatal(err)
	}
	return callsMarket{
		asset:  market.Asset{ID: market.AssetIDFrom(id), Symbol: "AAPLx", Mint: mint, Decimals: 6},
		price:  market.Price{Micros: money.MicrosFromUint64(price), ObservedAt: observed},
		priced: price > 0,
	}
}

func (m callsMarket) AssetsByMints(_ context.Context, mints []string) ([]market.Asset, error) {
	for _, mint := range mints {
		if mint == callsMint {
			return []market.Asset{m.asset}, nil
		}
	}
	return nil, nil
}

func (m callsMarket) PricesAsOf(context.Context, []market.AssetID, time.Time) (map[market.AssetID]market.Price, error) {
	if !m.priced {
		return map[market.AssetID]market.Price{}, nil
	}
	return map[market.AssetID]market.Price{m.asset.ID: m.price}, nil
}

func (callsMarket) ListingPrices(
	context.Context, []market.ListingKey, time.Time,
) (map[market.ListingKey]market.ListingPrice, error) {
	return map[market.ListingKey]market.ListingPrice{}, nil
}

type callsBallots map[uuid.UUID][]governanceport.Ballot

func (b callsBallots) Ballots(_ context.Context, id ids.ProposalID) ([]governanceport.Ballot, error) {
	return b[id.UUID()], nil
}

func ballotOf(voter uuid.UUID, choice string, at time.Time) governanceport.Ballot {
	if choice == "" {
		return governanceport.Ballot{VoterID: ids.UserIDFrom(voter)}
	}
	return governanceport.Ballot{VoterID: ids.UserIDFrom(voter), Choice: &choice, CastAt: &at}
}

func (d deliverer) callsDeliver(t *testing.T, ports app.Ports, handler string, ev events.Event) bool {
	t.Helper()
	dup, err := d.callsTry(t, ports, handler, ev)
	if err != nil {
		t.Fatalf("Deliver(%s) = %v", handler, err)
	}
	return dup
}

func (d deliverer) callsTry(t *testing.T, ports app.Ports, handler string, ev events.Event) (bool, error) {
	t.Helper()
	id := d.event(t, ev)
	deps := module.Deps{Pool: d.pool, UoW: d.uow, IDs: d.gen, Clock: d.clock, Bus: d.conn}
	ctx := observability.WithEventID(t.Context(), ids.EventIDFrom(id))
	for _, c := range ranking.New(deps, ranking.WithPorts(ports)).Consumers() {
		for _, h := range c.Handlers {
			if h.Name == handler {
				return bus.Deliver(ctx, d.uow, d.clock, h, ids.EventIDFrom(id), ev)
			}
		}
	}
	t.Fatalf("handler %s is not registered", handler)
	return false, nil
}

type callRow struct {
	state, outcome, voidReason string
	kind, symbol               string
	assetID                    *uuid.UUID
	fill, marketRef            *int64
	decidedAt, resolveAt       *time.Time
}

func (d deliverer) call(t *testing.T, proposal uuid.UUID) callRow {
	t.Helper()
	var r callRow
	err := d.pool.QueryRow(t.Context(), `SELECT state, coalesce(outcome, ''), coalesce(void_reason, ''),
		coalesce(kind, ''), coalesce(symbol, ''), asset_id, fill_price_micros, market_ref_micros, decided_at, resolve_at
		FROM calls WHERE proposal_id = $1`, proposal).Scan(
		&r.state, &r.outcome, &r.voidReason, &r.kind, &r.symbol, &r.assetID, &r.fill, &r.marketRef, &r.decidedAt, &r.resolveAt)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (d deliverer) voter(t *testing.T, proposal, user uuid.UUID) (choice string, at *time.Time) {
	t.Helper()
	err := d.pool.QueryRow(t.Context(), `SELECT coalesce(choice, '<nil>'), choice_at
		FROM call_voters WHERE proposal_id = $1 AND user_id = $2`, proposal, user).Scan(&choice, &at)
	if err != nil {
		t.Fatal(err)
	}
	return choice, at
}

type callsWorld struct {
	d        deliverer
	ports    app.Ports
	proposal uuid.UUID
	cabal    uuid.UUID
	assetID  uuid.UUID
	alice    uuid.UUID
	bob      uuid.UUID
	opened   time.Time
}

func newCallsWorld(t *testing.T, price uint64) callsWorld {
	t.Helper()
	d := newDeliverer(t)
	w := callsWorld{
		d: d, proposal: d.gen.NewV7(), cabal: d.gen.NewV7(), assetID: d.gen.NewV7(),
		alice: d.gen.NewV7(), bob: d.gen.NewV7(), opened: d.clock.Now(),
	}
	w.ports = app.Ports{
		Market: newCallsMarket(t, w.assetID, price, d.clock.Now().Add(-time.Minute)),
		Ballots: callsBallots{w.proposal: {
			ballotOf(w.alice, "yes", w.opened.Add(time.Minute)), ballotOf(w.bob, "", time.Time{}),
		}},
	}
	return w
}

func (w callsWorld) created(t *testing.T) {
	t.Helper()
	w.d.callsDeliver(t, w.ports, "ranking.calls.proposal_created", events.ProposalCreated{
		V: 1, ProposalID: w.proposal, CabalID: w.cabal, Kind: "buy", Symbol: "AAPLx", Mint: callsMint,
	})
}

func (w callsWorld) passed(t *testing.T) {
	t.Helper()
	w.d.callsDeliver(t, w.ports, "ranking.calls.proposal_passed", events.ProposalPassed{
		V: 1, ProposalID: w.proposal, CabalID: w.cabal, Kind: "buy", Symbol: "AAPLx", Mint: callsMint,
	})
}

func (w callsWorld) filled(t *testing.T) {
	t.Helper()
	w.d.callsDeliver(t, w.ports, "ranking.calls.trade_confirmed", events.TradeConfirmed{
		V: 1, SwapID: w.d.gen.NewV7(), CabalID: w.cabal, Source: events.TradeSource{Kind: "proposal", ID: w.proposal},
		Action: "buy", Symbol: "AAPLx", InMint: chain.SolanaAddress(callsUSDC), InAmount: 716_520_000,
		OutMint: callsMint, OutAmount: 1_000_000, USDCMicros: money.MicrosFromUint64(716_520_000),
		ConfirmedAt: w.opened.Add(time.Hour),
	})
}

func TestCalls_createdStoresTheOpeningFactsAndEveryFrozenVoter(t *testing.T) {
	t.Parallel()
	w := newCallsWorld(t, 0)
	w.created(t)
	got := w.d.call(t, w.proposal)
	if got.state != "open" || got.kind != "buy" || got.symbol != "AAPLx" || got.assetID == nil ||
		*got.assetID != w.assetID {
		t.Fatalf("call = %+v, want an open buy of AAPLx on the catalog asset", got)
	}
	if choice, at := w.d.voter(t, w.proposal, w.alice); choice != "yes" || at == nil {
		t.Errorf("alice = %s %v, want yes with a time", choice, at)
	}
	if choice, at := w.d.voter(t, w.proposal, w.bob); choice != "<nil>" || at != nil {
		t.Errorf("bob = %s %v, want a row with no choice", choice, at)
	}
}

func TestCalls_createdSkipsRuleProposals(t *testing.T) {
	t.Parallel()
	w := newCallsWorld(t, 0)
	w.d.callsDeliver(t, w.ports, "ranking.calls.proposal_created", events.ProposalCreated{
		V: 1, ProposalID: w.proposal, CabalID: w.cabal, Kind: "rule", Symbol: "-", Mint: callsMint,
	})
	var n int
	if err := w.d.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM calls) + (SELECT count(*) FROM call_voters)`).
		Scan(&n); err != nil ||
		n != 0 {
		t.Fatalf("rows after a rule proposal = %d, %v, want 0", n, err)
	}
}

func TestCalls_fillPriceIsStoredWhicheverEventComesFirst(t *testing.T) {
	t.Parallel()
	for name, order := range map[string][]func(callsWorld, *testing.T){
		"fill then decision":  {callsWorld.created, callsWorld.filled, callsWorld.passed},
		"decision then fill":  {callsWorld.created, callsWorld.passed, callsWorld.filled},
		"fill before opening": {callsWorld.filled, callsWorld.passed, callsWorld.created},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := newCallsWorld(t, 700_000_000)
			for _, step := range order {
				step(w, t)
			}
			assertFilledAndDecided(t, w.d.call(t, w.proposal))
		})
	}
}

func assertFilledAndDecided(t *testing.T, got callRow) {
	t.Helper()
	if got.fill == nil || *got.fill != 716_520_000 {
		t.Fatalf("fill_price_micros = %v, want 716520000", got.fill)
	}
	if got.state != "decided" || got.outcome != "passed" || got.marketRef == nil || *got.marketRef != 700_000_000 {
		t.Fatalf("call = %+v, want decided passed with the market reference kept beside the fill", got)
	}
	if got.decidedAt == nil || got.resolveAt == nil || got.resolveAt.Sub(*got.decidedAt) != 24*time.Hour {
		t.Fatalf("resolve_at = %v from decided_at %v, want 24h later", got.resolveAt, got.decidedAt)
	}
}

func TestCalls_aSellFillUsesTheInMintDecimals(t *testing.T) {
	t.Parallel()
	w := newCallsWorld(t, 0)
	w.d.callsDeliver(t, w.ports, "ranking.calls.trade_confirmed", events.TradeConfirmed{
		V:        1,
		SwapID:   w.d.gen.NewV7(),
		CabalID:  w.cabal,
		Source:   events.TradeSource{Kind: "proposal", ID: w.proposal},
		Action:   "sell",
		Symbol:   "AAPLx",
		InMint:   callsMint,
		InAmount: 2_000_000,
		OutMint: chain.SolanaAddress(
			callsUSDC,
		),
		OutAmount:  400_000_000,
		USDCMicros: money.MicrosFromUint64(400_000_000),
	})
	if got := w.d.call(t, w.proposal); got.fill == nil || *got.fill != 200_000_000 {
		t.Fatalf("fill_price_micros = %v, want 200000000", got.fill)
	}
}

func TestCalls_aFillThatIsNotAProposalTradeWritesNothing(t *testing.T) {
	t.Parallel()
	w := newCallsWorld(t, 0)
	w.d.callsDeliver(t, w.ports, "ranking.calls.trade_confirmed", events.TradeConfirmed{
		V:          1,
		SwapID:     w.d.gen.NewV7(),
		CabalID:    w.cabal,
		Source:     events.TradeSource{Kind: "cash_out", ID: w.proposal},
		Action:     "sell",
		InMint:     callsMint,
		InAmount:   1,
		OutMint:    chain.SolanaAddress(callsUSDC),
		USDCMicros: money.MicrosFromUint64(1),
	})
	var n int
	if err := w.d.pool.QueryRow(t.Context(), `SELECT count(*) FROM calls`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("calls = %d, %v, want 0", n, err)
	}
}

func TestCalls_theNewestBallotWinsInAnyOrder(t *testing.T) {
	t.Parallel()
	w := newCallsWorld(t, 0)
	base := w.opened
	cast := func(choice string, at time.Time) {
		w.d.callsDeliver(t, w.ports, "ranking.calls.vote_cast", events.VoteCast{
			V: 1, ProposalID: w.proposal, CabalID: w.cabal, VoterID: w.bob, Choice: choice, CastAt: at,
		})
	}
	cast("no", base.Add(2*time.Minute))
	cast("yes", base.Add(time.Minute))
	if choice, at := w.d.voter(
		t,
		w.proposal,
		w.bob,
	); choice != "no" || at == nil ||
		!at.Equal(base.Add(2*time.Minute)) {
		t.Fatalf("after the reversed pair bob = %s %v, want no at +2m", choice, at)
	}
	w.ports.Ballots = callsBallots{w.proposal: {ballotOf(w.bob, "yes", base.Add(time.Minute))}}
	w.passed(t)
	cast("yes", base.Add(30*time.Second))
	if choice, at := w.d.voter(
		t,
		w.proposal,
		w.bob,
	); choice != "no" || at == nil ||
		!at.Equal(base.Add(2*time.Minute)) {
		t.Fatalf("after the decision and an old redelivery bob = %s %v, want no at +2m", choice, at)
	}
}

func TestCalls_withdrawnAndVoidedProposalsEndVoidAndStayVoid(t *testing.T) {
	t.Parallel()
	w := newCallsWorld(t, 0)
	w.created(t)
	w.d.callsDeliver(t, w.ports, "ranking.calls.proposal_withdrawn", events.ProposalWithdrawn{
		V: 1, ProposalID: w.proposal, CabalID: w.cabal,
	})
	w.passed(t)
	got := w.d.call(t, w.proposal)
	if got.state != "void" || got.voidReason != "withdrawn" || got.outcome != "passed" {
		t.Fatalf("call = %+v, want void for withdrawn that a late decision did not reopen", got)
	}
	other := w
	other.proposal = w.d.gen.NewV7()
	other.d.callsDeliver(t, other.ports, "ranking.calls.proposal_voided", events.ProposalVoided{
		V: 1, ProposalID: other.proposal, CabalID: other.cabal, ActorType: "admin", Reason: "Duplicate.",
	})
	if got := other.d.call(t, other.proposal); got.state != "void" || got.voidReason != "Duplicate." {
		t.Fatalf("voided call = %+v, want void with its reason", got)
	}
}

func TestCalls_failedAndExpiredStoreTheNoSideAndTheMarketReference(t *testing.T) {
	t.Parallel()
	for handler, ev := range map[string]func(w callsWorld) events.Event{
		"ranking.calls.proposal_failed": func(w callsWorld) events.Event {
			return events.ProposalFailed{V: 1, ProposalID: w.proposal, CabalID: w.cabal}
		},
		"ranking.calls.proposal_expired": func(w callsWorld) events.Event {
			return events.ProposalExpired{V: 1, ProposalID: w.proposal, CabalID: w.cabal}
		},
	} {
		t.Run(handler, func(t *testing.T) {
			t.Parallel()
			w := newCallsWorld(t, 650_000_000)
			w.created(t)
			w.d.callsDeliver(t, w.ports, handler, ev(w))
			got := w.d.call(t, w.proposal)
			if got.state != "decided" || got.marketRef == nil || *got.marketRef != 650_000_000 {
				t.Fatalf("call = %+v, want decided with the market price at the decision", got)
			}
			var side string
			if err := w.d.pool.QueryRow(t.Context(), `SELECT side FROM cabal_calls WHERE proposal_id = $1`, w.proposal).
				Scan(&side); err != nil ||
				side != "no" {
				t.Fatalf("cabal side = %q, %v, want no", side, err)
			}
		})
	}
}

func TestCalls_aFailedProposalWithoutItsOpeningRowHasNoMarketReference(t *testing.T) {
	t.Parallel()
	w := newCallsWorld(t, 650_000_000)
	w.d.callsDeliver(
		t,
		w.ports,
		"ranking.calls.proposal_failed",
		events.ProposalFailed{V: 1, ProposalID: w.proposal, CabalID: w.cabal},
	)
	got := w.d.call(t, w.proposal)
	if got.state != "decided" || got.marketRef != nil || got.kind != "" {
		t.Fatalf("call = %+v, want decided with no kind and no market reference", got)
	}
}

func TestCalls_redeliveryOfAnEventWritesNothingTwice(t *testing.T) {
	t.Parallel()
	w := newCallsWorld(t, 650_000_000)
	ev := events.ProposalPassed{
		V:          1,
		ProposalID: w.proposal,
		CabalID:    w.cabal,
		Kind:       "buy",
		Symbol:     "AAPLx",
		Mint:       callsMint,
	}
	id := w.d.event(t, ev)
	deps := module.Deps{Pool: w.d.pool, UoW: w.d.uow, IDs: w.d.gen, Clock: w.d.clock, Bus: w.d.conn}
	var spec bus.HandlerSpec
	for _, c := range ranking.New(deps, ranking.WithPorts(w.ports)).Consumers() {
		for _, h := range c.Handlers {
			if h.Name == "ranking.calls.proposal_passed" {
				spec = h
			}
		}
	}
	ctx := observability.WithEventID(t.Context(), ids.EventIDFrom(id))
	for i, wantDup := range []bool{false, true} {
		dup, err := bus.Deliver(ctx, w.d.uow, w.d.clock, spec, ids.EventIDFrom(id), ev)
		if err != nil || dup != wantDup {
			t.Fatalf("delivery %d = duplicate %v, %v, want %v", i, dup, err, wantDup)
		}
	}
}

func seedAndresMonth(t *testing.T, d deliverer, period domain.Period, andres, cabal uuid.UUID) {
	t.Helper()
	otherCabal := d.gen.NewV7()
	symbols := []string{
		"AAPLx",
		"TSLAx",
		"NVDAx",
		"MSFTx",
		"GOOGLx",
		"METAx",
		"COINx",
		"HOODx",
		"SPYx",
		"SpaceX",
		"AMZN",
	}
	for i, symbol := range symbols {
		proposal := d.gen.NewV7()
		opened := period.Start.Add(time.Duration(i+1) * time.Hour)
		state, resolved := "resolved", opened.Add(48*time.Hour)
		if i >= 9 {
			state, resolved = "decided", opened
		}
		insertCall(t, d, proposal, cabal, symbol, state, opened, resolved)
		insertCallVoter(t, d, proposal, andres, cabal, i >= 9, i < 7)
	}
	noise := []struct {
		symbol, state string
		cabal         uuid.UUID
		opened        time.Time
		resolved      time.Time
	}{
		{"VOID", "void", cabal, period.Start.Add(time.Minute), period.Start},
		{"OLD", "resolved", cabal, period.Start.Add(-72 * time.Hour), period.Start.Add(-24 * time.Hour)},
		{"ELSE", "resolved", otherCabal, period.Start.Add(time.Hour), period.Start.Add(2 * time.Hour)},
	}
	for _, n := range noise {
		proposal := d.gen.NewV7()
		insertCall(t, d, proposal, n.cabal, n.symbol, n.state, n.opened, n.resolved)
		insertCallVoter(t, d, proposal, andres, n.cabal, false, true)
	}
}

func TestCallRecords_seededMonthForAndres(t *testing.T) {
	t.Parallel()
	d := newDeliverer(t)
	period := domain.PeriodContaining(domain.PeriodMonth, d.clock.Now())
	andres, cabal := d.gen.NewV7(), d.gen.NewV7()
	seedAndresMonth(t, d, period, andres, cabal)

	calls := ranking.New(module.Deps{Pool: d.pool}).Calls()
	user := ids.UserIDFrom(andres)
	scoped := ids.CabalIDFrom(cabal)
	records := func(cabal *ids.CabalID, p domain.Period) port.CallRecord {
		t.Helper()
		got, err := calls.Records(t.Context(), []ids.UserID{user}, cabal, p)
		if err != nil {
			t.Fatal(err)
		}
		return got[user]
	}
	if r := records(nil, period); r.Right != 8 || r.Resolved != 10 || r.Voted != 12 || r.Eligible != 12 {
		t.Fatalf("all cabals = %+v, want right 8 resolved 10 voted 12 eligible 12", r)
	}
	r := records(&scoped, period)
	if r.Right != 7 || r.Resolved != 9 || r.Voted != 11 || r.Eligible != 11 {
		t.Fatalf("one cabal = %+v, want right 7 resolved 9 voted 11 eligible 11", r)
	}
	if !slices.Equal(r.PendingSymbols, []string{"SpaceX", "AMZN"}) {
		t.Fatalf("PendingSymbols = %v, want [SpaceX AMZN]", r.PendingSymbols)
	}
	if all := records(nil, domain.PeriodContaining(domain.PeriodAll, d.clock.Now())); all.Resolved != 11 {
		t.Fatalf("all time = %+v, want 11 resolved", all)
	}
}

func TestCallRecords_cabalRecordsCountResolvedDecisions(t *testing.T) {
	t.Parallel()
	d := newDeliverer(t)
	period := domain.PeriodContaining(domain.PeriodMonth, d.clock.Now())
	cabal, quiet := d.gen.NewV7(), d.gen.NewV7()
	for i, right := range []bool{true, true, false} {
		proposal := d.gen.NewV7()
		insertCall(
			t,
			d,
			proposal,
			cabal,
			"AAPLx",
			"resolved",
			period.Start.Add(time.Hour),
			period.Start.Add(time.Duration(i+2)*time.Hour),
		)
		if _, err := d.pool.Exec(
			t.Context(),
			`INSERT INTO cabal_calls (proposal_id, cabal_id, side, is_right) VALUES ($1, $2, 'yes', $3)`,
			proposal,
			cabal,
			right,
		); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ranking.New(module.Deps{Pool: d.pool}).Calls().CabalRecords(
		t.Context(), []ids.CabalID{ids.CabalIDFrom(cabal), ids.CabalIDFrom(quiet)}, period)
	if err != nil {
		t.Fatal(err)
	}
	if r := got[ids.CabalIDFrom(cabal)]; r.Right != 2 || r.Resolved != 3 {
		t.Fatalf("cabal record = %+v, want right 2 resolved 3", r)
	}
	if r, ok := got[ids.CabalIDFrom(quiet)]; !ok || r.Resolved != 0 {
		t.Fatalf("quiet cabal = %+v, %v, want a zero record", r, ok)
	}
}

func insertCall(
	t *testing.T,
	d deliverer,
	proposal, cabal uuid.UUID,
	symbol, state string,
	opened, resolved time.Time,
) {
	t.Helper()
	var resolvedAt *time.Time
	if state == "resolved" {
		resolvedAt = &resolved
	}
	if _, err := d.pool.Exec(
		t.Context(),
		`INSERT INTO calls (proposal_id, cabal_id, kind, symbol, opened_at, resolved_at, state, updated_at)
		VALUES ($1, $2, 'buy', $3, $4, $5, $6, $4)`,
		proposal,
		cabal,
		symbol,
		opened,
		resolvedAt,
		state,
	); err != nil {
		t.Fatal(err)
	}
}

func insertCallVoter(t *testing.T, d deliverer, proposal, user, cabal uuid.UUID, open, right bool) {
	t.Helper()
	var judged *bool
	if !open {
		judged = &right
	}
	if _, err := d.pool.Exec(
		t.Context(),
		`INSERT INTO call_voters (proposal_id, user_id, cabal_id, choice, choice_at, is_right)
		VALUES ($1, $2, $3, 'yes', now(), $4)`,
		proposal,
		user,
		cabal,
		judged,
	); err != nil {
		t.Fatal(err)
	}
}

func TestCallRecords_readFailuresComeBack(t *testing.T) {
	t.Parallel()
	d := newDeliverer(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	calls := ranking.New(module.Deps{Pool: d.pool}).Calls()
	period := domain.PeriodContaining(domain.PeriodMonth, d.clock.Now())
	user, cabal := ids.UserIDFrom(d.gen.NewV7()), ids.CabalIDFrom(d.gen.NewV7())
	if _, err := calls.Records(ctx, []ids.UserID{user}, nil, period); err == nil {
		t.Error("Records on a cancelled context error = nil")
	}
	if _, err := calls.CabalRecords(ctx, []ids.CabalID{cabal}, period); err == nil {
		t.Error("CabalRecords on a cancelled context error = nil")
	}
}

type failingMarket struct{ callsMarket }

func (failingMarket) AssetsByMints(context.Context, []string) ([]market.Asset, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "test.market")
}

type failingBallots struct{}

func (failingBallots) Ballots(context.Context, ids.ProposalID) ([]governanceport.Ballot, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "test.ballots")
}

func TestCalls_readFailuresNakInsteadOfWritingHalfACall(t *testing.T) {
	t.Parallel()
	w := newCallsWorld(t, 0)
	created := events.ProposalCreated{V: 1, ProposalID: w.proposal, CabalID: w.cabal, Kind: "buy", Mint: callsMint}
	ports := w.ports
	ports.Market = failingMarket{}
	if _, err := w.d.callsTry(t, ports, "ranking.calls.proposal_created", created); err == nil {
		t.Error("created with a failing catalog error = nil")
	}
	ports = w.ports
	ports.Ballots = failingBallots{}
	if _, err := w.d.callsTry(t, ports, "ranking.calls.proposal_created", created); err == nil {
		t.Error("created with failing ballots error = nil")
	}
	bad := w.ports
	bad.Ballots = callsBallots{w.proposal: {ballotOf(w.alice, "maybe", w.opened)}}
	if _, err := w.d.callsTry(t, bad, "ranking.calls.proposal_created", created); err == nil {
		t.Error("created with an invalid stored choice error = nil")
	}
	var n int
	if err := w.d.pool.QueryRow(t.Context(), `SELECT count(*) FROM calls`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("calls after the failures = %d, %v, want 0", n, err)
	}
}

func TestCalls_aFillPriceBeyondBigintIsRefused(t *testing.T) {
	t.Parallel()
	w := newCallsWorld(t, 0)
	fill := events.TradeConfirmed{
		V: 1, SwapID: w.d.gen.NewV7(), CabalID: w.cabal, Source: events.TradeSource{Kind: "proposal", ID: w.proposal},
		Action: "buy", OutMint: callsMint, OutAmount: 1_000_000, USDCMicros: money.MicrosFromUint64(1 << 63),
	}
	if _, err := w.d.callsTry(t, w.ports, "ranking.calls.trade_confirmed", fill); err == nil {
		t.Error("a fill price above int64 error = nil")
	}
}

func TestCalls_aVoidedProposalWithoutAReasonIsStoredAsVoided(t *testing.T) {
	t.Parallel()
	w := newCallsWorld(t, 0)
	w.d.callsDeliver(t, w.ports, "ranking.calls.proposal_voided", events.ProposalVoided{
		V: 1, ProposalID: w.proposal, CabalID: w.cabal,
	})
	if got := w.d.call(t, w.proposal); got.voidReason != "voided" {
		t.Fatalf("void_reason = %q, want voided", got.voidReason)
	}
}

func TestCalls_fillsTheCallCannotUseAreSkippedOrRefused(t *testing.T) {
	t.Parallel()
	w := newCallsWorld(t, 0)
	fill := func(mutate func(*events.TradeConfirmed)) events.TradeConfirmed {
		e := events.TradeConfirmed{
			V:          1,
			SwapID:     w.d.gen.NewV7(),
			CabalID:    w.cabal,
			Source:     events.TradeSource{Kind: "proposal", ID: w.proposal},
			Action:     "buy",
			OutMint:    callsMint,
			OutAmount:  1_000_000,
			USDCMicros: money.MicrosFromUint64(5_000_000),
		}
		mutate(&e)
		return e
	}
	for name, ev := range map[string]events.TradeConfirmed{
		"unknown asset":  fill(func(e *events.TradeConfirmed) { e.OutMint = chain.SolanaAddress(callsUSDC) }),
		"unknown action": fill(func(e *events.TradeConfirmed) { e.Action = "swap" }),
	} {
		if _, err := w.d.callsTry(t, w.ports, "ranking.calls.trade_confirmed", ev); err != nil {
			t.Errorf("%s: error = %v, want a skip", name, err)
		}
	}
	zero := fill(func(e *events.TradeConfirmed) { e.OutAmount = 0 })
	if _, err := w.d.callsTry(t, w.ports, "ranking.calls.trade_confirmed", zero); err == nil {
		t.Error("a fill with no tokens error = nil")
	}
	failing := w.ports
	failing.Market = failingMarket{}
	if _, err := w.d.callsTry(
		t,
		failing,
		"ranking.calls.trade_confirmed",
		fill(func(*events.TradeConfirmed) {}),
	); err == nil {
		t.Error("a fill with a failing catalog error = nil")
	}
	var n int
	if err := w.d.pool.QueryRow(t.Context(), `SELECT count(*) FROM calls`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("calls after the skipped fills = %d, %v, want 0", n, err)
	}
}

func TestCalls_aMarketReferenceBeyondBigintIsRefused(t *testing.T) {
	t.Parallel()
	w := newCallsWorld(t, 1<<63)
	passed := events.ProposalPassed{V: 1, ProposalID: w.proposal, CabalID: w.cabal, Kind: "buy", Mint: callsMint}
	if _, err := w.d.callsTry(t, w.ports, "ranking.calls.proposal_passed", passed); err == nil {
		t.Error("a market reference above int64 error = nil")
	}
}

func TestCallRecords_aFailingPendingReadComesBack(t *testing.T) {
	t.Parallel()
	d := newDeliverer(t)
	if _, err := d.pool.Exec(t.Context(), `ALTER TABLE calls RENAME COLUMN symbol TO symbol_gone`); err != nil {
		t.Fatal(err)
	}
	store := adapters.NewCallStore(d.pool)
	period := domain.PeriodContaining(domain.PeriodMonth, d.clock.Now())
	user := ids.UserIDFrom(d.gen.NewV7())
	if _, err := app.NewCallRecords(store).Records(t.Context(), []ids.UserID{user}, nil, period); err == nil {
		t.Fatal("Records with a failing pending read error = nil")
	}
}

type pricesFailMarket struct{ callsMarket }

func (pricesFailMarket) PricesAsOf(
	context.Context,
	[]market.AssetID,
	time.Time,
) (map[market.AssetID]market.Price, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "test.prices")
}

func TestCalls_decisionFailuresReturnAnErrorAndAnUnhandledRuleProposalIsSkipped(t *testing.T) {
	t.Parallel()
	w := newCallsWorld(t, 700_000_000)
	passed := events.ProposalPassed{V: 1, ProposalID: w.proposal, CabalID: w.cabal, Kind: "buy", Mint: callsMint}
	failed := events.ProposalFailed{V: 1, ProposalID: w.proposal, CabalID: w.cabal}
	try := func(ports app.Ports, handler string, ev events.Event) error {
		_, err := w.d.callsTry(t, ports, handler, ev)
		return err
	}
	rule := passed
	rule.Kind = "rule"
	if err := try(w.ports, "ranking.calls.proposal_passed", rule); err != nil {
		t.Errorf("a passed rule proposal error = %v, want a skip", err)
	}
	noPrice := w.ports
	noPrice.Market = pricesFailMarket{newCallsMarket(t, w.assetID, 1, w.opened)}
	if err := try(noPrice, "ranking.calls.proposal_passed", passed); err == nil {
		t.Error("a decision with a failing price read error = nil")
	}
	noCatalog := w.ports
	noCatalog.Market = failingMarket{}
	if err := try(noCatalog, "ranking.calls.proposal_passed", passed); err == nil {
		t.Error("a decision with a failing catalog error = nil")
	}
	noBallots := w.ports
	noBallots.Ballots = failingBallots{}
	if err := try(noBallots, "ranking.calls.proposal_passed", passed); err == nil {
		t.Error("a decision with failing ballots error = nil")
	}
	badBallot := w.ports
	badBallot.Ballots = callsBallots{w.proposal: {ballotOf(w.alice, "maybe", w.opened)}}
	if err := try(badBallot, "ranking.calls.proposal_passed", passed); err == nil {
		t.Error("a decision with an invalid stored choice error = nil")
	}
	for _, ddl := range []string{
		`ALTER TABLE calls RENAME COLUMN outcome TO outcome_gone`,
		`ALTER TABLE calls RENAME COLUMN asset_id TO asset_id_gone`,
		`ALTER TABLE calls RENAME COLUMN kind TO kind_gone`,
	} {
		if _, err := w.d.pool.Exec(t.Context(), ddl); err != nil {
			t.Fatal(err)
		}
	}
	if err := try(w.ports, "ranking.calls.proposal_passed", passed); err == nil {
		t.Error("a decision the table refuses error = nil")
	}
	created := events.ProposalCreated{V: 1, ProposalID: w.proposal, CabalID: w.cabal, Kind: "buy", Mint: callsMint}
	if err := try(w.ports, "ranking.calls.proposal_created", created); err == nil {
		t.Error("an opening the table refuses error = nil")
	}
	if err := try(w.ports, "ranking.calls.proposal_failed", failed); err == nil {
		t.Error("a failed proposal whose call row cannot be read error = nil")
	}
}
