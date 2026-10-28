package ranking_test

import (
	"context"
	"errors"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const usdcMint = "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"

type ballotsFake map[uuid.UUID][]governanceport.Ballot

func (f ballotsFake) Ballots(_ context.Context, id ids.ProposalID) ([]governanceport.Ballot, error) {
	return f[id.UUID()], nil
}

type brokenBallots struct{ err error }

func (b brokenBallots) Ballots(context.Context, ids.ProposalID) ([]governanceport.Ballot, error) {
	return nil, b.err
}

type callsMarket struct {
	*marketfake.CatalogFake
	*marketfake.PricesFake
}

func (m callsMarket) AssetsByMints(ctx context.Context, mints []string) ([]market.Asset, error) {
	return m.ByMints(ctx, mints)
}

func (callsMarket) ListingPrices(
	context.Context, []market.ListingKey, time.Time,
) (map[market.ListingKey]market.ListingPrice, error) {
	return map[market.ListingKey]market.ListingPrice{}, nil
}

type callsRig struct {
	deliverer
	ballots     ballotsFake
	ballotsDown error
	catalog     *marketfake.CatalogFake
	prices      *marketfake.PricesFake
	cabal       uuid.UUID
	proposal    uuid.UUID
	voters      [3]uuid.UUID
}

func newCallsRig(t *testing.T) *callsRig {
	t.Helper()
	d := newDeliverer(t)
	prices := &marketfake.PricesFake{}
	prices.Set(marketfake.TSLAx().ID, money.MicrosFromUint64(700_000_000), d.clock.Now().Add(-time.Hour))
	prices.Set(marketfake.SPACEX().ID, money.MicrosFromUint64(190_000_000), d.clock.Now().Add(-time.Hour))
	r := &callsRig{
		deliverer: d, ballots: ballotsFake{}, prices: prices,
		catalog: marketfake.NewCatalog(append(marketfake.Fixtures(), marketfake.SPACEX())...),
		cabal:   d.gen.NewV7(), proposal: d.gen.NewV7(),
		voters: [3]uuid.UUID{d.gen.NewV7(), d.gen.NewV7(), d.gen.NewV7()},
	}
	return r
}

func (r *callsRig) handler(t *testing.T, name string) bus.HandlerSpec {
	t.Helper()
	deps := module.Deps{Pool: r.pool, UoW: r.uow, IDs: r.gen, Clock: r.clock}
	ballots := app.Ballots(r.ballots)
	if r.ballotsDown != nil {
		ballots = brokenBallots{err: r.ballotsDown}
	}
	ports := app.Ports{Market: callsMarket{r.catalog, r.prices}, Ballots: ballots}
	for _, c := range ranking.New(deps, ranking.WithPorts(ports)).Consumers() {
		for _, h := range c.Handlers {
			if h.Name == name {
				return h
			}
		}
	}
	t.Fatalf("handler %s is not registered", name)
	return bus.HandlerSpec{}
}

func (r *callsRig) deliver(t *testing.T, handler string, ev events.Event) uuid.UUID {
	t.Helper()
	id := r.event(t, ev)
	r.redeliver(t, id, handler, ev, false)
	return id
}

func (r *callsRig) redeliver(t *testing.T, id uuid.UUID, handler string, ev events.Event, wantDuplicate bool) {
	t.Helper()
	duplicate, err := bus.Deliver(t.Context(), r.uow, r.clock, r.handler(t, handler), ids.EventIDFrom(id), ev)
	if err != nil || duplicate != wantDuplicate {
		t.Fatalf(
			"%s delivery = duplicate %t, %v; want duplicate %t and no error",
			handler,
			duplicate,
			err,
			wantDuplicate,
		)
	}
}

func (r *callsRig) created(kind string, mint string) events.ProposalCreated {
	return events.ProposalCreated{
		V: 1, ProposalID: r.proposal, CabalID: r.cabal, ProposerID: r.voters[0], Kind: kind,
		Symbol: "TSLAx", Mint: chain.SolanaAddress(mint), USDCMicros: money.MicrosFromUint64(25_000_000),
		VoterCount: len(r.voters),
	}
}

func (r *callsRig) passed() events.ProposalPassed {
	return events.ProposalPassed{
		V: 1, ProposalID: r.proposal, CabalID: r.cabal, Kind: "buy", Symbol: "TSLAx",
		Mint: chain.SolanaAddress(marketfake.TSLAx().Mint.String()),
	}
}

func (r *callsRig) buyFill(price uint64) events.TradeConfirmed {
	return events.TradeConfirmed{
		V: 1, SwapID: r.gen.NewV7(), CabalID: r.cabal, Source: events.TradeSource{Kind: "proposal", ID: r.proposal},
		Action: "buy", Symbol: "TSLAx", InMint: usdcMint, InAmount: price,
		OutMint: chain.SolanaAddress(marketfake.TSLAx().Mint.String()), OutAmount: 100_000_000,
		USDCMicros: money.MicrosFromUint64(price), ConfirmedAt: r.clock.Now(),
	}
}

type callRow struct {
	State                                                         string
	Kind, Outcome, VoidReason                                     *string
	AssetID                                                       *uuid.UUID
	OpenedAt, DecidedAt, ResolveAt, FilledAt, MarketRefObservedAt *time.Time
	MarketRef, FillPrice                                          *int64
}

func (r *callsRig) call(t *testing.T) callRow {
	t.Helper()
	var row callRow
	err := r.pool.QueryRow(t.Context(), `
		SELECT state, kind, outcome, void_reason, asset_id, opened_at, decided_at, resolve_at, filled_at,
			market_ref_observed_at, market_ref_micros, fill_price_micros
		FROM calls WHERE proposal_id = $1`, r.proposal).Scan(
		&row.State, &row.Kind, &row.Outcome, &row.VoidReason, &row.AssetID, &row.OpenedAt, &row.DecidedAt,
		&row.ResolveAt, &row.FilledAt, &row.MarketRefObservedAt, &row.MarketRef, &row.FillPrice)
	if err != nil {
		t.Fatalf("read call %s: %v", r.proposal, err)
	}
	return row
}

func (r *callsRig) noCall(t *testing.T) {
	t.Helper()
	var n int
	err := r.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM calls) + (SELECT count(*) FROM call_voters)`).
		Scan(&n)
	if err != nil || n != 0 {
		t.Fatalf("calls and call_voters hold %d rows, %v; want none", n, err)
	}
}

type voterRow struct {
	Choice   *string
	ChoiceAt *time.Time
}

func (r *callsRig) voterRows(t *testing.T) map[uuid.UUID]voterRow {
	t.Helper()
	rows, err := r.pool.Query(
		t.Context(),
		`SELECT user_id, choice, choice_at FROM call_voters WHERE proposal_id = $1`,
		r.proposal,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[uuid.UUID]voterRow{}
	for rows.Next() {
		var (
			user uuid.UUID
			v    voterRow
		)
		if err := rows.Scan(&user, &v.Choice, &v.ChoiceAt); err != nil {
			t.Fatal(err)
		}
		out[user] = v
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (r *callsRig) cabalSide(t *testing.T) string {
	t.Helper()
	var side string
	if err := r.pool.QueryRow(t.Context(), `SELECT side FROM cabal_calls WHERE proposal_id = $1`, r.proposal).
		Scan(&side); err != nil {
		t.Fatalf("read cabal call: %v", err)
	}
	return side
}

func ballot(user uuid.UUID, choice string, at time.Time) governanceport.Ballot {
	b := governanceport.Ballot{VoterID: ids.UserIDFrom(user)}
	if choice != "" {
		b.Choice, b.CastAt = &choice, &at
	}
	return b
}

func TestCalls_aFillIsKeptWhicheverOfTheFillAndTheDecisionArrivesFirst(t *testing.T) {
	t.Parallel()
	for name, fillFirst := range map[string]bool{"fill first": true, "decision first": false} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newCallsRig(t)
			sampled := r.clock.Now().Add(-time.Hour)
			r.ballots[r.proposal] = []governanceport.Ballot{ballot(r.voters[0], "yes", r.clock.Now())}
			fill := r.buyFill(716_520_000)
			r.deliver(t, "ranking.calls.proposal_created", r.created("buy", marketfake.TSLAx().Mint.String()))
			r.clock.Advance(time.Minute)
			if fillFirst {
				r.deliver(t, "ranking.calls.trade_confirmed", fill)
			}
			r.clock.Advance(time.Minute)
			decision := r.passed()
			decisionID := r.deliver(t, "ranking.calls.proposal_passed", decision)
			if !fillFirst {
				r.deliver(t, "ranking.calls.trade_confirmed", fill)
			}
			r.redeliver(t, decisionID, "ranking.calls.proposal_passed", decision, true)

			got := r.call(t)
			assertPassedWithFill(t, got, fill)
			assertMarketRef(t, got, 700_000_000, sampled)
			assertDecisionClock(t, got, r.clock.Now())
			if side := r.cabalSide(t); side != "yes" {
				t.Fatalf("cabal side = %s, want yes for a passed proposal", side)
			}
		})
	}
}

func assertPassedWithFill(t *testing.T, got callRow, fill events.TradeConfirmed) {
	t.Helper()
	if got.State != "decided" || got.Outcome == nil || *got.Outcome != "passed" {
		t.Fatalf("call = %+v, want decided as passed", got)
	}
	if got.FillPrice == nil || *got.FillPrice != 716_520_000 {
		t.Fatalf("fill price = %v, want 716520000 whichever event came first", got.FillPrice)
	}
	if got.FilledAt == nil || !got.FilledAt.Equal(fill.ConfirmedAt) {
		t.Fatalf("filled at = %v, want the trade's confirmation time %s", got.FilledAt, fill.ConfirmedAt)
	}
}

func assertMarketRef(t *testing.T, got callRow, micros int64, observed time.Time) {
	t.Helper()
	if got.MarketRef == nil || *got.MarketRef != micros {
		t.Fatalf("market ref = %v, want %d", got.MarketRef, micros)
	}
	if got.MarketRefObservedAt == nil || !got.MarketRefObservedAt.Equal(observed) {
		t.Fatalf("market ref observed at = %v, want the sample's time %s", got.MarketRefObservedAt, observed)
	}
}

func assertDecisionClock(t *testing.T, got callRow, decidedAt time.Time) {
	t.Helper()
	if got.DecidedAt == nil || !got.DecidedAt.Equal(decidedAt) {
		t.Fatalf("decided at = %v, want the decision's delivery time %s", got.DecidedAt, decidedAt)
	}
	if got.ResolveAt == nil || !got.ResolveAt.Equal(decidedAt.Add(24*time.Hour)) {
		t.Fatalf("resolve at = %v, want 24 hours after the decision", got.ResolveAt)
	}
}

func TestCalls_aSellFillUsesTheInMintDecimals(t *testing.T) {
	t.Parallel()
	r := newCallsRig(t)
	spacex := marketfake.SPACEX()
	fill := events.TradeConfirmed{
		V: 1, SwapID: r.gen.NewV7(), CabalID: r.cabal, Source: events.TradeSource{Kind: "proposal", ID: r.proposal},
		Action: "sell", Symbol: "SPACEX", InMint: chain.SolanaAddress(spacex.Mint.String()), InAmount: 2_000_000_000,
		OutMint: usdcMint, OutAmount: 385_200_000, USDCMicros: money.MicrosFromUint64(385_200_000),
		ConfirmedAt: r.clock.Now(),
	}

	r.deliver(t, "ranking.calls.trade_confirmed", fill)

	if got := r.call(t); got.FillPrice == nil || *got.FillPrice != 192_600_000 || got.State != "open" {
		t.Fatalf("call = %+v, want a 192600000 fill priced with the 9 decimals of the mint sold, still open", got)
	}
}

func TestCalls_aFillIsIgnoredWhenItIsNotAProposalTradeAndFailsWhenTheAssetIsUnknown(t *testing.T) {
	t.Parallel()
	r := newCallsRig(t)
	cashout := r.buyFill(1_000_000)
	cashout.Source.Kind = "cashout"
	r.deliver(t, "ranking.calls.trade_confirmed", cashout)
	r.noCall(t)

	unknown := r.buyFill(1_000_000)
	unknown.OutMint = usdcMint
	id := r.event(t, unknown)
	_, err := bus.Deliver(
		t.Context(),
		r.uow,
		r.clock,
		r.handler(t, "ranking.calls.trade_confirmed"),
		ids.EventIDFrom(id),
		unknown,
	)
	if errs.CodeOf(err) != errs.CodeAssetNotFound {
		t.Fatalf("fill of a mint outside the catalog err = %v, want asset_not_found", err)
	}
	r.noCall(t)
}

func TestCalls_theNewestBallotWinsWhateverTheDeliveryOrder(t *testing.T) {
	t.Parallel()
	r := newCallsRig(t)
	older, newer := r.clock.Now().Add(-2*time.Minute), r.clock.Now().Add(-time.Minute)
	voter := r.voters[0]
	yes := events.VoteCast{V: 1, ProposalID: r.proposal, CabalID: r.cabal, VoterID: voter, Choice: "yes", CastAt: older}
	no := events.VoteCast{V: 1, ProposalID: r.proposal, CabalID: r.cabal, VoterID: voter, Choice: "no", CastAt: newer}

	r.deliver(t, "ranking.calls.vote_cast", no)
	r.deliver(t, "ranking.calls.vote_cast", yes)
	assertVoter(t, r.voterRows(t)[voter], "no", newer, "the later ballot, delivered first, survives the earlier one")

	r.ballots[r.proposal] = []governanceport.Ballot{
		ballot(voter, "no", newer), ballot(r.voters[1], "", time.Time{}), ballot(r.voters[2], "yes", older),
	}
	r.deliver(t, "ranking.calls.proposal_passed", r.passed())
	stale := events.VoteCast{
		V:          1,
		ProposalID: r.proposal,
		CabalID:    r.cabal,
		VoterID:    voter,
		Choice:     "yes",
		CastAt:     older.Add(time.Second),
	}
	r.deliver(t, "ranking.calls.vote_cast", stale)

	rows := r.voterRows(t)
	assertVoter(t, rows[voter], "no", newer, "an older vote.cast delivered after the decision changes nothing")
	if none := rows[r.voters[1]]; none.Choice != nil || none.ChoiceAt != nil {
		t.Errorf("frozen voter who never voted = %+v, want a row with no choice", none)
	}
	assertVoter(t, rows[r.voters[2]], "yes", older, "the decision records every final ballot")
}

func assertVoter(t *testing.T, got voterRow, choice string, at time.Time, why string) {
	t.Helper()
	if got.Choice == nil || *got.Choice != choice || got.ChoiceAt == nil || !got.ChoiceAt.Equal(at) {
		t.Errorf("voter = %v at %v, want %s at %s: %s", got.Choice, got.ChoiceAt, choice, at, why)
	}
}

func TestCalls_aNonVotingBallotNeverErasesAChoiceThatArrivedFirst(t *testing.T) {
	t.Parallel()
	r := newCallsRig(t)
	at := r.clock.Now().Add(-time.Minute)
	voter := r.voters[0]
	r.deliver(t, "ranking.calls.vote_cast",
		events.VoteCast{V: 1, ProposalID: r.proposal, CabalID: r.cabal, VoterID: voter, Choice: "yes", CastAt: at})
	r.ballots[r.proposal] = []governanceport.Ballot{ballot(voter, "", time.Time{})}

	r.deliver(t, "ranking.calls.proposal_expired",
		events.ProposalExpired{V: 1, ProposalID: r.proposal, CabalID: r.cabal})

	assertVoter(
		t,
		r.voterRows(t)[voter],
		"yes",
		at,
		"a ballot snapshot taken before the vote committed must not erase it",
	)
	if side := r.cabalSide(t); side != "no" {
		t.Fatalf("cabal side = %s, want no for an expired proposal", side)
	}
}

func TestCalls_openingRecordsEveryFrozenVoterAndSkipsRuleProposals(t *testing.T) {
	t.Parallel()
	r := newCallsRig(t)
	at := r.clock.Now().Add(-time.Minute)
	r.ballots[r.proposal] = []governanceport.Ballot{
		ballot(r.voters[0], "no", at), ballot(r.voters[1], "", time.Time{}), ballot(r.voters[2], "", time.Time{}),
	}
	r.deliver(t, "ranking.calls.proposal_created", r.created("rule", marketfake.TSLAx().Mint.String()))
	r.noCall(t)

	r.deliver(t, "ranking.calls.proposal_created", r.created("buy", marketfake.TSLAx().Mint.String()))

	got := r.call(t)
	if got.State != "open" || got.Kind == nil || *got.Kind != "buy" || got.AssetID == nil ||
		*got.AssetID != marketfake.TSLAx().ID.UUID() || got.OpenedAt == nil || !got.OpenedAt.Equal(r.clock.Now()) {
		t.Fatalf("call = %+v, want an open buy on TSLAx opened at the delivery time", got)
	}
	rows := r.voterRows(t)
	if len(rows) != 3 || rows[r.voters[1]].Choice != nil {
		t.Fatalf("voters = %+v, want the three frozen voters and no choice for the two who have not voted", rows)
	}
	assertVoter(t, rows[r.voters[0]], "no", at, "a ballot cast before the opening event was seen")
}

func TestCalls_anUnlistedMintOpensTheCallWithoutAnAsset(t *testing.T) {
	t.Parallel()
	r := newCallsRig(t)

	r.deliver(t, "ranking.calls.proposal_created", r.created("buy", usdcMint))

	if got := r.call(t); got.AssetID != nil || got.State != "open" {
		t.Fatalf("call = %+v, want an open call with no asset", got)
	}
}

func TestCalls_aFailedOrExpiredProposalIsPricedFromItsOpeningRow(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		handler string
		ev      func(r *callsRig) events.Event
		outcome string
	}{
		"failed": {"ranking.calls.proposal_failed", func(r *callsRig) events.Event {
			return events.ProposalFailed{V: 1, ProposalID: r.proposal, CabalID: r.cabal}
		}, "failed"},
		"expired": {"ranking.calls.proposal_expired", func(r *callsRig) events.Event {
			return events.ProposalExpired{V: 1, ProposalID: r.proposal, CabalID: r.cabal}
		}, "expired"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newCallsRig(t)
			r.deliver(t, "ranking.calls.proposal_created", r.created("buy", marketfake.TSLAx().Mint.String()))
			r.clock.Advance(time.Hour)

			r.deliver(t, tc.handler, tc.ev(r))

			got := r.call(t)
			if got.State != "decided" || got.Outcome == nil || *got.Outcome != tc.outcome {
				t.Fatalf("call = %+v, want decided as %s", got, tc.outcome)
			}
			if got.MarketRef == nil || *got.MarketRef != 700_000_000 {
				t.Fatalf("market ref = %v, want the asset's price at the decision", got.MarketRef)
			}
			if side := r.cabalSide(t); side != "no" {
				t.Fatalf("cabal side = %s, want no", side)
			}
		})
	}
}

func TestCalls_aFailedProposalWithoutItsOpeningRowHasNoMarketReference(t *testing.T) {
	t.Parallel()
	r := newCallsRig(t)

	r.deliver(t, "ranking.calls.proposal_failed", events.ProposalFailed{V: 1, ProposalID: r.proposal, CabalID: r.cabal})

	got := r.call(t)
	if got.State != "decided" || got.MarketRef != nil || got.MarketRefObservedAt != nil || got.Kind != nil {
		t.Fatalf("call = %+v, want a decided call with no kind and no market reference for the resolver to void", got)
	}
	r.deliver(t, "ranking.calls.proposal_created", r.created("buy", marketfake.TSLAx().Mint.String()))
	if got := r.call(t); got.State != "decided" || got.Kind == nil || got.MarketRef != nil {
		t.Fatalf("call after the opening = %+v, want the opening columns filled and the decision untouched", got)
	}
}

func TestCalls_aStaleMarketPriceIsStoredWithItsObservationTime(t *testing.T) {
	t.Parallel()
	r := newCallsRig(t)
	r.clock.Advance(72 * time.Hour)

	r.deliver(t, "ranking.calls.proposal_passed", r.passed())

	got := r.call(t)
	want := r.clock.Now().Add(-73 * time.Hour)
	if got.MarketRefObservedAt == nil || !got.MarketRefObservedAt.Equal(want) {
		t.Fatalf("observed at = %v, want %s so the resolver can judge freshness", got.MarketRefObservedAt, want)
	}
}

func TestCalls_aWithdrawnOrVoidedProposalIsVoidAndStaysVoid(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		handler string
		ev      func(r *callsRig) events.Event
		reason  string
	}{
		"withdrawn": {"ranking.calls.proposal_withdrawn", func(r *callsRig) events.Event {
			return events.ProposalWithdrawn{V: 1, ProposalID: r.proposal, CabalID: r.cabal, ProposerID: r.voters[0]}
		}, "withdrawn"},
		"voided": {"ranking.calls.proposal_voided", func(r *callsRig) events.Event {
			return events.ProposalVoided{V: 1, ProposalID: r.proposal, CabalID: r.cabal, ActorType: "admin", Reason: "Duplicate."}
		}, "Duplicate."},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newCallsRig(t)
			r.deliver(t, tc.handler, tc.ev(r))
			if got := r.call(t); got.State != "void" || got.VoidReason == nil || *got.VoidReason != tc.reason {
				t.Fatalf("call = %+v, want void with reason %q", got, tc.reason)
			}

			r.deliver(t, "ranking.calls.proposal_created", r.created("buy", marketfake.TSLAx().Mint.String()))
			r.deliver(t, "ranking.calls.proposal_passed", r.passed())

			got := r.call(t)
			if got.State != "void" || got.VoidReason == nil || *got.VoidReason != tc.reason {
				t.Fatalf("call after a late decision = %+v, want it to stay void", got)
			}
			if got.DecidedAt == nil || got.FillPrice != nil {
				t.Fatalf("call = %+v, want the decision recorded without reopening the call", got)
			}
		})
	}
}

func TestCalls_aDecidedCallThatIsVoidedLaterIsVoid(t *testing.T) {
	t.Parallel()
	r := newCallsRig(t)
	r.deliver(t, "ranking.calls.proposal_passed", r.passed())

	r.deliver(t, "ranking.calls.proposal_voided",
		events.ProposalVoided{V: 1, ProposalID: r.proposal, CabalID: r.cabal, ActorType: "admin", Reason: "Banned."})

	if got := r.call(t); got.State != "void" || got.DecidedAt == nil {
		t.Fatalf("call = %+v, want void with the decision still recorded", got)
	}
}

func TestCalls_aResolvedCallIgnoresALateFillAndVoid(t *testing.T) {
	t.Parallel()
	r := newCallsRig(t)
	r.deliver(t, "ranking.calls.proposal_passed", r.passed())
	if _, err := r.pool.Exec(
		t.Context(),
		`UPDATE calls SET state = 'resolved' WHERE proposal_id = $1`,
		r.proposal,
	); err != nil {
		t.Fatal(err)
	}

	r.deliver(t, "ranking.calls.trade_confirmed", r.buyFill(716_520_000))
	r.deliver(t, "ranking.calls.proposal_voided",
		events.ProposalVoided{V: 1, ProposalID: r.proposal, CabalID: r.cabal, ActorType: "admin", Reason: "Banned."})

	if got := r.call(t); got.State != "resolved" || got.FillPrice != nil || got.VoidReason != nil {
		t.Fatalf("call = %+v, want a resolved call that a late fill or void does not touch", got)
	}
}

func TestCalls_aLaterFillReplacesAnEarlierOneButNeverTheOtherWayRound(t *testing.T) {
	t.Parallel()
	r := newCallsRig(t)
	first, second := r.buyFill(700_000_000), r.buyFill(716_520_000)
	first.ConfirmedAt, second.ConfirmedAt = r.clock.Now().Add(-time.Minute), r.clock.Now()

	r.deliver(t, "ranking.calls.trade_confirmed", second)
	r.deliver(t, "ranking.calls.trade_confirmed", first)

	if got := r.call(t); got.FillPrice == nil || *got.FillPrice != 716_520_000 {
		t.Fatalf("fill = %v, want the newest fill whichever was delivered last", got.FillPrice)
	}
}

func TestCalls_resettingAChoiceKeepsTheRowAndRejectsOlderBallots(t *testing.T) {
	t.Parallel()
	r := newCallsRig(t)
	q := sqlc.New(r.pool)
	voter := r.voters[0]
	t1, t2, t3 := r.clock.Now().Add(-3*time.Minute), r.clock.Now().Add(-2*time.Minute), r.clock.Now().Add(-time.Minute)
	cast := func(choice string, at time.Time) {
		t.Helper()
		err := q.UpsertCallVoter(t.Context(), sqlc.UpsertCallVoterParams{
			ProposalID: r.proposal,
			UserID:     voter,
			CabalID:    r.cabal,
			Choice:     pgtype.Text{String: choice, Valid: true},
			ChoiceAt:   pgtype.Timestamptz{Time: at, Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	reset := func(at time.Time) int64 {
		t.Helper()
		n, err := q.ResetCallVoterChoice(t.Context(), sqlc.ResetCallVoterChoiceParams{
			ProposalID: r.proposal, UserID: voter, At: pgtype.Timestamptz{Time: at, Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	cast("yes", t1)

	if n := reset(t2); n != 1 {
		t.Fatalf("reset after the ballot changed %d rows, want 1", n)
	}
	if n := reset(t1); n != 0 {
		t.Fatalf("a reset older than the stored time changed %d rows, want 0", n)
	}
	cast("yes", t1)
	got := r.voterRows(t)[voter]
	if got.Choice != nil || got.ChoiceAt == nil || !got.ChoiceAt.Equal(t2) {
		t.Fatalf("voter after a reset and an older ballot = %+v, want no choice, kept at the reset time", got)
	}
	cast("no", t3)
	assertVoter(t, r.voterRows(t)[voter], "no", t3, "a ballot newer than the reset counts again")
}

func seedAndresMonth(t *testing.T, r *callsRig, month domain.Period) (pending []string) {
	t.Helper()
	other := r.gen.NewV7()
	pendingSymbols := map[int]string{9: "SPACEX", 10: "AMZNx"}
	for i := range 11 {
		call := seedCall{
			id: r.gen.NewV7(), cabal: r.cabal, symbol: "AAPLx", state: "resolved",
			opened: month.Start.Add(time.Duration(i+1) * time.Hour),
		}
		if i >= 7 {
			call.cabal = other
		}
		if symbol, open := pendingSymbols[i]; open {
			call.state, call.symbol = "decided", symbol
			pending = append(pending, symbol)
		} else {
			call.resolved = month.Start.Add(time.Duration(i+1) * 24 * time.Hour)
			call.right = ptr(i < 7)
		}
		seedCallRow(t, r, call, r.voters[0], "yes")
	}
	return pending
}

func seedMonthNoise(t *testing.T, r *callsRig, month domain.Period) {
	t.Helper()
	andres, other := r.voters[0], r.voters[1]
	seedCallRow(t, r, seedCall{
		id: r.gen.NewV7(), cabal: r.cabal, symbol: "TSLAx", state: "resolved", right: ptr(true),
		opened: month.Start.Add(-72 * time.Hour), resolved: month.Start.Add(-48 * time.Hour),
	}, andres, "yes")
	seedCallRow(t, r, seedCall{
		id: r.gen.NewV7(), cabal: r.cabal, symbol: "TSLAx", state: "void", opened: month.Start.Add(time.Hour),
	}, andres, "yes")
	voted := seedCall{
		id: r.gen.NewV7(), cabal: r.cabal, symbol: "TSLAx", state: "decided", opened: month.Start.Add(2 * time.Hour),
	}
	seedCallRow(t, r, voted, other, "no")
	skipped := seedCall{
		id: r.gen.NewV7(), cabal: r.cabal, symbol: "TSLAx", state: "decided", opened: month.Start.Add(3 * time.Hour),
	}
	seedCallRow(t, r, skipped, other, "")
}

func (r *callsRig) period(t *testing.T, kind domain.PeriodKind) domain.Period {
	t.Helper()
	p, err := domain.PeriodContaining(kind, r.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (r *callsRig) records(
	t *testing.T, users []uuid.UUID, cabal *ids.CabalID, p domain.Period,
) map[ids.UserID]port.CallRecord {
	t.Helper()
	who := make([]ids.UserID, len(users))
	for i, u := range users {
		who[i] = ids.UserIDFrom(u)
	}
	got, err := ranking.New(module.Deps{Pool: r.pool}).Calls().Records(t.Context(), who, cabal, p)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func assertRecord(t *testing.T, who string, got port.CallRecord, right, resolved, voted, eligible int) {
	t.Helper()
	if got.Right != right || got.Resolved != resolved || got.Voted != voted || got.Eligible != eligible {
		t.Fatalf("%s = %+v, want right %d, resolved %d, voted %d, eligible %d",
			who, got, right, resolved, voted, eligible)
	}
}

func TestCalls_recordsCountRightResolvedAndTurnoutForAMonth(t *testing.T) {
	t.Parallel()
	r := newCallsRig(t)
	month := r.period(t, domain.PeriodMonth)
	pending := seedAndresMonth(t, r, month)
	seedMonthNoise(t, r, month)

	got := r.records(t, r.voters[:], nil, month)

	andres := got[ids.UserIDFrom(r.voters[0])]
	assertRecord(t, "Andres", andres, 7, 9, 11, 11)
	if !slices.Equal(andres.PendingSymbols, pending) {
		t.Fatalf("Andres pending = %v, want %v: the votes that count once they resolve", andres.PendingSymbols, pending)
	}
	assertRecord(t, "a member who skipped one proposal", got[ids.UserIDFrom(r.voters[1])], 0, 0, 1, 2)
	carol, ok := got[ids.UserIDFrom(r.voters[2])]
	if !ok {
		t.Fatal("a member with no calls has no entry, want a zero record")
	}
	assertRecord(t, "a member with no calls", carol, 0, 0, 0, 0)
}

func TestCalls_recordsScopeToOneCabal(t *testing.T) {
	t.Parallel()
	r := newCallsRig(t)
	month := r.period(t, domain.PeriodMonth)
	seedAndresMonth(t, r, month)
	seedMonthNoise(t, r, month)
	cabal := ids.CabalIDFrom(r.cabal)

	got := r.records(t, r.voters[:1], &cabal, month)

	assertRecord(t, "Andres in one cabal", got[ids.UserIDFrom(r.voters[0])], 7, 7, 7, 7)
}

func TestCalls_recordsForAllTimeIncludeEarlierMonths(t *testing.T) {
	t.Parallel()
	r := newCallsRig(t)
	month := r.period(t, domain.PeriodMonth)
	seedAndresMonth(t, r, month)
	seedMonthNoise(t, r, month)

	got := r.records(t, r.voters[:1], nil, r.period(t, domain.PeriodAll))

	andres := got[ids.UserIDFrom(r.voters[0])]
	if andres.Right != 8 || andres.Resolved != 10 {
		t.Fatalf("Andres all time = %+v, want the September call counted too: right 8 of 10 resolved", andres)
	}
}

func TestCalls_recordsOfNobodyIsEmpty(t *testing.T) {
	t.Parallel()
	r := newCallsRig(t)

	if got := r.records(t, nil, nil, r.period(t, domain.PeriodMonth)); len(got) != 0 {
		t.Fatalf("Records of nobody = %v, want none", got)
	}
}

func TestCalls_cabalRecordsCountTheCabalsOwnCalls(t *testing.T) {
	t.Parallel()
	r := newCallsRig(t)
	month, _ := domain.PeriodContaining(domain.PeriodMonth, r.clock.Now())
	quiet := r.gen.NewV7()
	for i, right := range []*bool{ptr(true), ptr(true), ptr(false), nil} {
		call := seedCall{id: r.gen.NewV7(), cabal: r.cabal, symbol: "AAPLx", state: "decided", opened: month.Start}
		if right != nil {
			call.state, call.resolved, call.right = "resolved", month.Start.Add(time.Duration(i+1)*time.Hour), right
		}
		seedCabalCall(t, r, call, "yes")
	}
	calls := ranking.New(module.Deps{Pool: r.pool}).Calls()

	got, err := calls.CabalRecords(t.Context(), []ids.CabalID{ids.CabalIDFrom(r.cabal), ids.CabalIDFrom(quiet)}, month)
	if err != nil {
		t.Fatal(err)
	}
	if rec := got[ids.CabalIDFrom(r.cabal)]; rec.Right != 2 || rec.Resolved != 3 {
		t.Fatalf("cabal = %+v, want 2 of 3 resolved right, the unresolved one not counted", rec)
	}
	if rec, ok := got[ids.CabalIDFrom(quiet)]; !ok || rec.Resolved != 0 {
		t.Fatalf("cabal with no calls = %+v, %t, want a zero record present", rec, ok)
	}
}

type seedCall struct {
	id, cabal        uuid.UUID
	symbol, state    string
	opened, resolved time.Time
	right            *bool
}

func ptr[T any](v T) *T { return &v }

func nilIfZero(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func seedCallRow(t *testing.T, r *callsRig, c seedCall, user uuid.UUID, choice string) {
	t.Helper()
	insertCall(t, r, c)
	var chosen *string
	var chosenAt *time.Time
	if choice != "" {
		chosen, chosenAt = &choice, ptr(c.opened)
	}
	_, err := r.pool.Exec(t.Context(), `
		INSERT INTO call_voters (proposal_id, user_id, cabal_id, choice, choice_at, "right")
		VALUES ($1, $2, $3, $4, $5, $6)`, c.id, user, c.cabal, chosen, chosenAt, c.right)
	if err != nil {
		t.Fatal(err)
	}
}

func seedCabalCall(t *testing.T, r *callsRig, c seedCall, side string) {
	t.Helper()
	insertCall(t, r, c)
	_, err := r.pool.Exec(t.Context(), `
		INSERT INTO cabal_calls (proposal_id, cabal_id, side, "right") VALUES ($1, $2, $3, $4)`,
		c.id, c.cabal, side, c.right)
	if err != nil {
		t.Fatal(err)
	}
}

func insertCall(t *testing.T, r *callsRig, c seedCall) {
	t.Helper()
	_, err := r.pool.Exec(t.Context(), `
		INSERT INTO calls (proposal_id, cabal_id, kind, symbol, opened_at, resolved_at, state, updated_at)
		VALUES ($1, $2, 'buy', $3, $4, $5, $6, $7) ON CONFLICT (proposal_id) DO NOTHING`,
		c.id, c.cabal, c.symbol, c.opened, nilIfZero(c.resolved), c.state, r.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
}

func TestCalls_aMarketReadFailureFailsTheDeliveryAndRecordsNothing(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		fault, handler string
		ev             func(r *callsRig) events.Event
	}{
		"the asset lookup for an opening": {"ByMints", "ranking.calls.proposal_created", func(r *callsRig) events.Event {
			return r.created("buy", marketfake.TSLAx().Mint.String())
		}},
		"the asset lookup for a decision": {"ByMints", "ranking.calls.proposal_passed", func(r *callsRig) events.Event {
			return r.passed()
		}},
		"the asset lookup for a fill": {"ByMints", "ranking.calls.trade_confirmed", func(r *callsRig) events.Event {
			return r.buyFill(716_520_000)
		}},
		"the price at the decision": {"PricesAsOf", "ranking.calls.proposal_passed", func(r *callsRig) events.Event {
			return r.passed()
		}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newCallsRig(t)
			down := errs.New(errs.CodeUpstreamUnavailable, "test.market")
			if tc.fault == "ByMints" {
				r.catalog.Fail(tc.fault, down)
			} else {
				r.prices.Fail(tc.fault, down)
			}
			ev := tc.ev(r)
			id := r.event(t, ev)

			_, err := bus.Deliver(t.Context(), r.uow, r.clock, r.handler(t, tc.handler), ids.EventIDFrom(id), ev)

			if !errors.Is(err, down) {
				t.Fatalf("delivery err = %v, want the market failure so the consumer retries", err)
			}
			r.noCall(t)
		})
	}
}

func TestCalls_aFailingReadSurfacesAsAnErrorFromEachRecordQuery(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		breakTable, breakColumn string
		read                    func(r *callsRig, c port.Calls) error
	}{
		"the member counts": {"call_voters", "choice", func(r *callsRig, c port.Calls) error {
			_, err := c.Records(t.Context(), []ids.UserID{ids.UserIDFrom(r.voters[0])}, nil, r.period(t, domain.PeriodMonth))
			return err
		}},
		"the pending symbols": {"calls", "symbol", func(r *callsRig, c port.Calls) error {
			_, err := c.Records(t.Context(), []ids.UserID{ids.UserIDFrom(r.voters[0])}, nil, r.period(t, domain.PeriodMonth))
			return err
		}},
		"the cabal counts": {"cabal_calls", `"right"`, func(r *callsRig, c port.Calls) error {
			_, err := c.CabalRecords(t.Context(), []ids.CabalID{ids.CabalIDFrom(r.cabal)}, r.period(t, domain.PeriodMonth))
			return err
		}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newCallsRig(t)
			if _, err := r.pool.Exec(t.Context(),
				"ALTER TABLE "+tc.breakTable+" RENAME COLUMN "+tc.breakColumn+" TO renamed_away"); err != nil {
				t.Fatal(err)
			}

			err := tc.read(r, app.NewCallRecords(r.pool))

			if errs.CodeOf(err) != errs.CodeInternal {
				t.Fatalf("read err = %v, want an internal error", err)
			}
		})
	}
}

func TestCalls_cabalRecordsOfNoCabalsIsEmpty(t *testing.T) {
	t.Parallel()
	r := newCallsRig(t)

	got, err := ranking.New(module.Deps{Pool: r.pool}).
		Calls().
		CabalRecords(t.Context(), nil, r.period(t, domain.PeriodMonth))

	if err != nil || len(got) != 0 {
		t.Fatalf("CabalRecords of nobody = %v, %v, want none", got, err)
	}
}

func TestCalls_aProposalThatIsNotAnOrdinaryTradePassingRecordsNothing(t *testing.T) {
	t.Parallel()
	r := newCallsRig(t)
	rule := r.passed()
	rule.Kind = "rule"

	r.deliver(t, "ranking.calls.proposal_passed", rule)

	r.noCall(t)
}

func TestCalls_aBallotsFailureFailsEveryHandlerThatReadsThem(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		handler string
		ev      func(r *callsRig) events.Event
	}{
		"opening": {"ranking.calls.proposal_created", func(r *callsRig) events.Event {
			return r.created("buy", marketfake.TSLAx().Mint.String())
		}},
		"a pass": {"ranking.calls.proposal_passed", func(r *callsRig) events.Event { return r.passed() }},
		"a failure": {"ranking.calls.proposal_failed", func(r *callsRig) events.Event {
			return events.ProposalFailed{V: 1, ProposalID: r.proposal, CabalID: r.cabal}
		}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newCallsRig(t)
			r.ballotsDown = errs.New(errs.CodeDBUnavailable, "test.ballots")
			ev := tc.ev(r)
			id := r.event(t, ev)

			_, err := bus.Deliver(t.Context(), r.uow, r.clock, r.handler(t, tc.handler), ids.EventIDFrom(id), ev)

			if errs.CodeOf(err) != errs.CodeDBUnavailable {
				t.Fatalf("delivery err = %v, want db_unavailable so the consumer retries", err)
			}
			r.noCall(t)
		})
	}
}

func TestCalls_aFailedProposalWhoseCallCannotBeReadFailsTheDelivery(t *testing.T) {
	t.Parallel()
	r := newCallsRig(t)
	if _, err := r.pool.Exec(t.Context(), `ALTER TABLE calls RENAME COLUMN asset_id TO renamed_away`); err != nil {
		t.Fatal(err)
	}
	ev := events.ProposalFailed{V: 1, ProposalID: r.proposal, CabalID: r.cabal}
	id := r.event(t, ev)

	_, err := bus.Deliver(
		t.Context(),
		r.uow,
		r.clock,
		r.handler(t, "ranking.calls.proposal_failed"),
		ids.EventIDFrom(id),
		ev,
	)

	if errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("delivery err = %v, want db_unavailable so the consumer retries", err)
	}
}

func TestCalls_aWriteThatCannotBeExecutedIsReturnedFromTheTransaction(t *testing.T) {
	t.Parallel()
	hugePrice := money.MicrosFromUint64(math.MaxUint64)
	ballots := []governanceport.Ballot{ballot(uuid.Nil, "yes", time.Time{})}
	for name, tc := range map[string]struct {
		cancelled bool
		apply     func(r *callsRig, ctx context.Context, tx db.Tx, at time.Time) error
	}{
		"the opening row": {true, func(r *callsRig, ctx context.Context, tx db.Tx, at time.Time) error {
			return adapters.Calls{}.ApplyCreated(ctx, tx, r.created("buy", usdcMint), app.CallOpening{}, at)
		}},
		"the ballots of a decision": {true, func(r *callsRig, ctx context.Context, tx db.Tx, at time.Time) error {
			return adapters.Calls{}.ApplyPassed(ctx, tx, r.passed(), app.CallDecision{Ballots: ballots}, at)
		}},
		"the decision row": {true, func(r *callsRig, ctx context.Context, tx db.Tx, at time.Time) error {
			return adapters.Calls{}.ApplyPassed(ctx, tx, r.passed(), app.CallDecision{}, at)
		}},
		"a market price past what the column holds": {false, func(r *callsRig, ctx context.Context, tx db.Tx, at time.Time) error {
			ref := &app.MarketRef{Micros: hugePrice, ObservedAt: at}
			return adapters.Calls{}.ApplyPassed(ctx, tx, r.passed(), app.CallDecision{Ref: ref}, at)
		}},
		"a fill price past what the column holds": {false, func(r *callsRig, ctx context.Context, tx db.Tx, at time.Time) error {
			return adapters.Calls{}.ApplyFill(ctx, tx, r.buyFill(1), app.CallFill{Price: hugePrice}, at)
		}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newCallsRig(t)
			applyCtx, cancel := context.WithCancel(t.Context())
			if tc.cancelled {
				cancel()
			}
			defer cancel()

			err := r.uow.Do(observability.WithActor(t.Context(), "system:ranking-test"),
				func(_ context.Context, tx db.Tx) error { return tc.apply(r, applyCtx, tx, r.clock.Now()) })

			if err == nil {
				t.Fatal("transaction committed, want the failed write returned")
			}
			r.noCall(t)
		})
	}
}
