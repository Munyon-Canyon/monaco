package notify_test

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/domain"
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
	agentKind    = "agent"

	metaTokens = 42_000_000
	metaUSDC   = 300_938_400
	voted      = "{vote}"
)

func fixtureCatalog() *marketfake.CatalogFake {
	return marketfake.NewCatalog(append(marketfake.Fixtures(), marketfake.SPACEX())...)
}

func insertRealAsset(t *testing.T, r *pushRig, a market.Asset) {
	t.Helper()
	r.exec(t, `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name, issuer_tradable,
		company_key, first_seen_at, updated_at, chain_checked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, true, $8, $9, $9, $9)`,
		a.ID.UUID(), a.Symbol, a.Mint.String(), int16(a.Decimals), string(a.Issuer), string(a.Kind), a.DisplayName,
		a.CompanyKey, r.clock.Now())
}

type resultWorld struct {
	proposalWorld
	choices map[ids.UserID]string
	idle    ids.UserID
}

func (r *pushRig) resultWorldOf(t *testing.T, yes, no int) resultWorld {
	t.Helper()
	w := resultWorld{proposalWorld: r.proposalWorldOf(t, yes+no+1), choices: map[ids.UserID]string{}}
	var ballots []governanceport.Ballot
	for i, user := range w.members[:yes+no] {
		if i < yes {
			ballots, w.choices[user] = append(ballots, yesBallot(user)), "Yes"
			continue
		}
		ballots, w.choices[user] = append(ballots, noBallot(user)), "No"
	}
	w.idle = w.members[yes+no]
	w.ports.ballots.set(w.refs.proposal, append(ballots, eligibleBallot(w.idle))...)
	return w
}

func (w resultWorld) voters() []ids.UserID {
	return slices.DeleteFunc(slices.Clone(w.members), func(u ids.UserID) bool { return w.choices[u] == "" })
}

func (w resultWorld) source() events.TradeSource {
	return events.TradeSource{Kind: proposalKind, ID: w.refs.proposal}
}

func (r *pushRig) confirmedEvent(
	t *testing.T,
	w resultWorld,
	action string,
	tokens, usdc uint64,
) events.TradeConfirmed {
	t.Helper()
	e := goldenEvent(t, events.TypeTradeConfirmed).(events.TradeConfirmed)
	e.SwapID, e.CabalID, e.Source, e.Action, e.Symbol = r.ids.NewV7(), w.id.UUID(), w.source(), action, "AAPLx"
	e.USDCMicros = money.MicrosFromUint64(usdc)
	e.InAmount, e.OutAmount = usdc, tokens
	if action == sellKind {
		e.InAmount, e.OutAmount = tokens, usdc
	}
	return e
}

func (r *pushRig) failedEvent(t *testing.T, w resultWorld, action string) events.TradeFailed {
	t.Helper()
	e := goldenEvent(t, events.TypeTradeFailed).(events.TradeFailed)
	e.SwapID, e.CabalID, e.Source, e.Action, e.Symbol = r.ids.NewV7(), w.id.UUID(), w.source(), action, "AAPLx"
	return e
}

func (r *pushRig) blockedEvent(t *testing.T, w resultWorld, action string) events.TradeBlocked {
	t.Helper()
	e := goldenEvent(t, events.TypeTradeBlocked).(events.TradeBlocked)
	e.CabalID, e.Source, e.Action, e.Symbol, e.Code = w.id.UUID(), w.source(), action, "AAPLx", errs.CodeNoRoute
	return e
}

func (w resultWorld) closedEvent(typ events.Type) events.Event {
	if typ == events.TypeProposalFailed {
		return events.ProposalFailed{V: 1, ProposalID: w.refs.proposal, CabalID: w.id.UUID()}
	}
	return events.ProposalExpired{V: 1, ProposalID: w.refs.proposal, CabalID: w.id.UUID()}
}

func (w resultWorld) handle(t *testing.T, r *pushRig, d bus.Delivery, e events.Event) error {
	t.Helper()
	p := w.ports.results()
	switch e := e.(type) {
	case events.TradeConfirmed:
		return handleKinds(t, r, r.sender, d, e, app.ResultFilled{ResultPorts: p})
	case events.TradeFailed:
		return handleKinds(t, r, r.sender, d, e, app.ResultFillFailed{ResultPorts: p})
	case events.TradeBlocked:
		return handleKinds(t, r, r.sender, d, e, app.ResultFillBlocked{ResultPorts: p})
	case events.ProposalFailed:
		return handleKinds(t, r, r.sender, d, e, app.ResultFailed{ResultPorts: p})
	case events.ProposalExpired:
		return handleKinds(t, r, r.sender, d, e, app.ResultExpired{ResultPorts: p})
	}
	t.Fatalf("no result kind for %T", e)
	return nil
}

func (w resultWorld) run(t *testing.T, r *pushRig, actor string, e events.Event) (bus.Delivery, error) {
	t.Helper()
	d := r.emit(t, actor, e)
	return d, w.handle(t, r, d, e)
}

func (w resultWorld) wantPushes(
	t *testing.T, r *pushRig, d bus.Delivery, title, body, outcome string, to ...ids.UserID,
) {
	t.Helper()
	byToken := func(a, b apns.Push) int { return strings.Compare(a.Token, b.Token) }
	want := make([]apns.Push, 0, len(to))
	states := map[ids.UserID]string{}
	for _, user := range to {
		states[user] = "delivered"
		want = append(want, inCabal(apns.Push{
			UserID: user, Token: w.tokens[user], Environment: apns.Sandbox,
			CollapseID: "proposal-" + w.refs.proposal.String(), Title: title,
			Body: strings.ReplaceAll(body, voted, w.choices[user]),
			Data: map[string]string{
				"kind": resultPush, "cabal_id": w.id.String(), "proposal_id": w.refs.proposal.String(),
				"outcome": outcome,
			},
		}, w.id, cabalSender(w.id)))
	}
	slices.SortFunc(want, byToken)
	sent := r.sender.Sent()
	slices.SortFunc(sent, byToken)
	if !reflect.DeepEqual(sent, want) {
		t.Fatalf("sent %+v, want one push per voter and nobody else: %+v", sent, want)
	}
	r.wantStates(t, d, states)
	r.wantRecorded(t, d, 1)
	r.wantCount(t, "rows of a retired kind", 0, `SELECT count(*) FROM notifications
		WHERE kind IN ('proposal_passed', 'trade_filled', 'trade_failed', 'trade_blocked')`)
}

func (r *pushRig) wantNobodyPushed(t *testing.T, d bus.Delivery, err error) {
	t.Helper()
	wantVerdict(t, err, "", errs.VerdictAck)
	if sent := r.sender.Sent(); len(sent) != 0 {
		t.Fatalf("sent %+v, want nothing", sent)
	}
	r.wantCount(t, "notification rows", 0, `SELECT count(*) FROM notifications`)
	r.wantRecorded(t, d, 1)
}

func TestNotify_ResultFilled_PushesTheVotersOnlyWithTheFill(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		action       string
		tokens, usdc uint64
		body         string
	}{
		"a buy":                    {buyKind, metaTokens, metaUSDC, "Bought 0.42 sh at $716.52. You voted {vote}."},
		"a sell":                   {sellKind, 220_000_000, 1_000_000_000, "Sold 2.2 sh at $454.55. You voted {vote}."},
		"the fixture's NVDA fill":  {buyKind, 182_000_000, 399_308_000, "Bought 1.82 sh at $219.40. You voted {vote}."},
		"a whole number of shares": {buyKind, 300_000_000, 1_500_000_000, "Bought 3 sh at $500.00. You voted {vote}."},
		"a fraction below a cent":  {buyKind, 1_000_000, 4_999, "Bought 0.01 sh at $0.50. You voted {vote}."},
		"a fill of no shares":      {buyKind, 0, 4_999, "Bought 0 sh at $0.00. You voted {vote}."},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			w := r.resultWorldOf(t, 4, 1)

			d, err := w.run(t, r, tradeActor, r.confirmedEvent(t, w, tc.action, tc.tokens, tc.usdc))

			wantVerdict(t, err, "", errs.VerdictAck)
			w.wantPushes(t, r, d, "AAPL passed 4–1", tc.body, "filled", w.voters()...)
		})
	}
}

func TestNotify_ResultFillFailed_PushesTheVotersOnly(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		event func(t *testing.T, r *pushRig, w resultWorld) events.Event
		body  string
	}{
		"a failed buy": {
			func(t *testing.T, r *pushRig, w resultWorld) events.Event {
				t.Helper()
				return r.failedEvent(t, w, buyKind)
			},
			"The buy didn't go through. No money moved. You voted {vote}.",
		},
		"a failed sell": {
			func(t *testing.T, r *pushRig, w resultWorld) events.Event {
				t.Helper()
				return r.failedEvent(t, w, sellKind)
			},
			"The sell didn't go through. No money moved. You voted {vote}.",
		},
		"a blocked buy": {
			func(t *testing.T, r *pushRig, w resultWorld) events.Event {
				t.Helper()
				return r.blockedEvent(t, w, buyKind)
			},
			"The buy didn't go through. No money moved. You voted {vote}.",
		},
		"a blocked sell": {
			func(t *testing.T, r *pushRig, w resultWorld) events.Event {
				t.Helper()
				return r.blockedEvent(t, w, sellKind)
			},
			"The sell didn't go through. The shares are still in the pot. You voted {vote}.",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			w := r.resultWorldOf(t, 4, 1)

			d, err := w.run(t, r, tradeActor, tc.event(t, r, w))

			wantVerdict(t, err, "", errs.VerdictAck)
			w.wantPushes(t, r, d, "AAPL passed 4–1", tc.body, "fill_failed", w.voters()...)
		})
	}
}

func TestNotify_ResultFailed_SkipsTheVoterWhoseBallotFailedIt(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.resultWorldOf(t, 2, 2)
	deciding := w.members[3]
	w.ports.heads.set(w.refs.proposal, governanceport.ProposalHead{Kind: sellKind, Symbol: "TSLAx"})

	d, err := w.run(t, r, userActor(deciding), w.closedEvent(events.TypeProposalFailed))

	wantVerdict(t, err, "", errs.VerdictAck)
	w.wantPushes(t, r, d, "TSLA failed 2–2", "No trade. You voted {vote}.", "failed", w.members[:3]...)
}

func TestNotify_ResultExpired_PushesTheVotersAndReadsAPreIPOByItsName(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.resultWorldOf(t, 1, 1)
	w.ports.heads.set(w.refs.proposal, governanceport.ProposalHead{Kind: buyKind, Symbol: "SPACEX"})

	d, err := w.run(t, r, "system:governance.expiry", w.closedEvent(events.TypeProposalExpired))

	wantVerdict(t, err, "", errs.VerdictAck)
	w.wantPushes(t, r, d, "SpaceX closed 1–1", "Not enough votes by the close. No trade. You voted {vote}.", "expired",
		w.voters()...)
}

func TestNotify_ResultExpired_AProposalWithNoBallotsPushesNobody(t *testing.T) {
	t.Parallel()
	for _, typ := range []events.Type{events.TypeProposalExpired, events.TypeProposalFailed} {
		t.Run(string(typ), func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			w := r.proposalWorldOf(t, 3)
			rw := resultWorld{proposalWorld: w}
			rw.ports.ballots.set(w.refs.proposal, eligibleBallot(w.members[1]), eligibleBallot(w.members[2]))

			d, err := rw.run(t, r, "system:governance.expiry", rw.closedEvent(typ))

			r.wantNobodyPushed(t, d, err)
		})
	}
}

func TestNotify_Result_ARuleProposalPushesNobody(t *testing.T) {
	t.Parallel()
	for _, typ := range []events.Type{events.TypeProposalExpired, events.TypeProposalFailed} {
		t.Run(string(typ), func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			w := r.resultWorldOf(t, 2, 1)
			w.ports.heads.set(w.refs.proposal, governanceport.ProposalHead{Kind: "rule"})

			d, err := w.run(t, r, "system:governance.expiry", w.closedEvent(typ))

			r.wantNobodyPushed(t, d, err)
		})
	}
}

func TestNotify_Result_AFailedAttemptThenARetryNotifiesTheVotersOncePerAttempt(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.resultWorldOf(t, 4, 1)
	failed, confirmed := r.failedEvent(t, w, buyKind), r.confirmedEvent(t, w, buyKind, metaTokens, metaUSDC)

	failedDelivery, failedErr := w.run(t, r, tradeActor, failed)
	confirmedDelivery, confirmedErr := w.run(t, r, tradeActor, confirmed)
	redeliveredErr := w.handle(t, r, confirmedDelivery, confirmed)

	wantVerdict(t, failedErr, "", errs.VerdictAck)
	wantVerdict(t, confirmedErr, "", errs.VerdictAck)
	wantVerdict(t, redeliveredErr, "", errs.VerdictAck)
	voters := len(w.voters())
	r.wantCount(t, "fill_failed rows", voters, `SELECT count(*) FROM notifications
		WHERE kind = 'proposal_result' AND data->>'outcome' = 'fill_failed' AND source_event_id = $1`,
		failedDelivery.EventID.UUID())
	r.wantCount(t, "filled rows", voters, `SELECT count(*) FROM notifications
		WHERE kind = 'proposal_result' AND data->>'outcome' = 'filled' AND source_event_id = $1`,
		confirmedDelivery.EventID.UUID())
	r.wantCount(t, "rows under the proposal's collapse id", 2*voters, `SELECT count(*) FROM notifications
		WHERE collapse_id = $1`, "proposal-"+w.refs.proposal.String())
	if sent := r.sender.Sent(); len(sent) != 2*voters {
		t.Fatalf(
			"sent %d pushes, want %d: the failure and then the fill for each voter, none twice",
			len(sent),
			2*voters,
		)
	}
}

func TestNotify_Result_ARedeliveredEventPushesEachVoterOnce(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.resultWorldOf(t, 4, 1)
	e := r.confirmedEvent(t, w, buyKind, metaTokens, metaUSDC)
	d := r.emit(t, tradeActor, e)

	first, second := w.handle(t, r, d, e), w.handle(t, r, d, e)

	wantVerdict(t, first, "", errs.VerdictAck)
	wantVerdict(t, second, "", errs.VerdictAck)
	if sent := r.sender.Sent(); len(sent) != len(w.voters()) {
		t.Fatalf("sent %d pushes after a redelivery, want one per voter of %d", len(sent), len(w.voters()))
	}
	r.wantCount(t, "rows", len(w.voters()), `SELECT count(*) FROM notifications`)
}

func fromSource(e events.Event, kind string) events.Event {
	switch e := e.(type) {
	case events.TradeConfirmed:
		e.Source.Kind = kind
		return e
	case events.TradeFailed:
		e.Source.Kind = kind
		return e
	case events.TradeBlocked:
		e.Source.Kind = kind
		return e
	}
	return e
}

func TestNotify_Trade_ANonProposalSourceNotifiesNobodyAndReadsNoPort(t *testing.T) {
	t.Parallel()
	for _, source := range []string{cashOutKind, agentKind} {
		for _, action := range []string{buyKind, sellKind} {
			t.Run(source+"/"+action, func(t *testing.T) {
				t.Parallel()
				r := newPushRig(t)
				w := r.resultWorldOf(t, 2, 1)
				down := errs.New(errs.CodeDBUnavailable, "test.port")
				for _, op := range []string{"Members", "Cabal", "Ballots", "ProposalHead", "AssetBySymbol"} {
					w.ports.fail(op, down)
				}
				for _, e := range []events.Event{
					r.confirmedEvent(t, w, action, metaTokens, metaUSDC), r.failedEvent(t, w, action),
					r.blockedEvent(t, w, action),
				} {
					d, err := w.run(t, r, tradeActor, fromSource(e, source))
					r.wantNobodyPushed(t, d, err)
				}
			})
		}
	}
}

func TestNotify_Result_APushNeedsAnAssetItCanFillInOnlyForTheFill(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.resultWorldOf(t, 4, 1)
	w.ports.assets = marketfake.NewCatalog()
	failed := r.failedEvent(t, w, buyKind)

	d, err := w.run(t, r, tradeActor, failed)

	wantVerdict(t, err, "", errs.VerdictAck)
	w.wantPushes(t, r, d, "a stock passed 4–1", "The buy didn't go through. No money moved. You voted {vote}.",
		"fill_failed", w.voters()...)
	_, err = w.run(t, r, tradeActor, r.confirmedEvent(t, w, buyKind, metaTokens, metaUSDC))
	wantVerdict(t, err, errs.CodeAssetNotFound, errs.VerdictTerm)
}

func TestNotify_Result_PortFailuresReturnAsIsAndWriteNothing(t *testing.T) {
	t.Parallel()
	reads := map[string][]string{
		"confirmed": {"Members", "Cabal", "Ballots", "AssetBySymbol"},
		"failed":    {"Members", "Cabal", "Ballots", "AssetBySymbol"},
		"blocked":   {"Members", "Cabal", "Ballots", "AssetBySymbol"},
		"closed":    {"Members", "Cabal", "Ballots", "ProposalHead", "AssetBySymbol"},
	}
	for typ, ops := range reads {
		for _, op := range ops {
			t.Run(typ+"/"+op, func(t *testing.T) {
				t.Parallel()
				r := newPushRig(t)
				w := r.resultWorldOf(t, 4, 1)
				down := errs.New(errs.CodeDBUnavailable, "test."+op)
				w.ports.fail(op, down)
				e := map[string]events.Event{
					"confirmed": r.confirmedEvent(t, w, buyKind, metaTokens, metaUSDC),
					"failed":    r.failedEvent(t, w, buyKind), "blocked": r.blockedEvent(t, w, buyKind),
					"closed": w.closedEvent(events.TypeProposalExpired),
				}[typ]

				d, err := w.run(t, r, tradeActor, e)

				if !errors.Is(err, down) {
					t.Fatalf("Handle = %v, want the port error itself, %v", err, down)
				}
				wantVerdict(t, err, errs.CodeDBUnavailable, errs.VerdictNak)
				r.wantCount(t, "notification rows", 0, `SELECT count(*) FROM notifications`)
				r.wantRecorded(t, d, 0)
			})
		}
	}
}

func TestNotify_Result_RefusesAnActionOrAChoiceItCannotPhraseAndWritesNothing(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.resultWorldOf(t, 4, 1)
	for name, e := range map[string]events.Event{
		"a swap that fills":  r.confirmedEvent(t, w, "swap", metaTokens, metaUSDC),
		"a swap that fails":  r.failedEvent(t, w, "swap"),
		"a swap that blocks": r.blockedEvent(t, w, "swap"),
	} {
		d, err := w.run(t, r, tradeActor, e)

		wantVerdict(t, err, errs.CodeInvalidInput, errs.VerdictTerm)
		r.wantCount(t, name+": notification rows", 0, `SELECT count(*) FROM notifications`)
		r.wantRecorded(t, d, 0)
	}
	maybe := "maybe"
	w.ports.ballots.set(w.refs.proposal, governanceport.Ballot{VoterID: w.members[0], Choice: &maybe})

	d, err := w.run(t, r, tradeActor, r.failedEvent(t, w, buyKind))

	wantVerdict(t, err, errs.CodeInvalidInput, errs.VerdictTerm)
	r.wantRecorded(t, d, 0)
}

func TestNotify_ResultRecipients_AreTheVotersWhoAreStillMembersInPortOrder(t *testing.T) {
	t.Parallel()
	g := testkit.NewIDs(1)
	cabalID := ids.CabalIDFrom(g.NewV7())
	first, second, third, idle, left := ids.NewUserID(g), ids.NewUserID(g), ids.NewUserID(g), ids.NewUserID(g),
		ids.NewUserID(g)
	joined := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	member := func(user ids.UserID, after time.Duration) fakes.CabalMember {
		return fakes.CabalMember{CabalID: cabalID, Member: cabal.MemberView{UserID: user, JoinedAt: joined.Add(after)}}
	}
	cabals := fakes.NewCabal(
		[]fakes.CabalSeed{{View: cabal.View{ID: cabalID, Name: cabalName}}},
		[]fakes.CabalMember{
			member(third, 0), member(first, 2*time.Minute), member(idle, 3*time.Minute), member(second, time.Minute),
		},
	)
	confirmed := goldenEvent(t, events.TypeTradeConfirmed).(events.TradeConfirmed)
	confirmed.CabalID, confirmed.Source.Kind = cabalID.UUID(), proposalKind
	ballots := (&ballotBox{}).set(confirmed.Source.ID,
		yesBallot(first), noBallot(left), yesBallot(third), eligibleBallot(idle), noBallot(second))
	ports := app.ResultPorts{Cabals: cabals, Ballots: ballots, Heads: &headBox{}}

	to, err := app.ResultFilled{ResultPorts: ports}.Recipients(t.Context(), confirmed)

	if want := []ids.UserID{third, second, first}; err != nil || !slices.Equal(to, want) {
		t.Fatalf("Recipients = %v, %v, want the voters who are still members, in port order, %v", to, err, want)
	}
}

func TestNotify_Result_DecisionRequestsOffSilencesBothKinds(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.resultWorldOf(t, 4, 1)
	silent := w.members[0]
	r.preference(t, silent, domain.CategoryDecisionRequests, false)
	e := r.confirmedEvent(t, w, buyKind, metaTokens, metaUSDC)
	d := r.emit(t, tradeActor, e)
	created := r.createdEvent(t, w.refs, buyKind, "AAPLx")
	createdDelivery := r.emit(t, userActor(w.members[1]), created)

	resultErr := w.handle(t, r, d, e)
	createdErr := handleKinds(t, r, r.sender, createdDelivery, created, w.ports.created(r))

	wantVerdict(t, resultErr, "", errs.VerdictAck)
	wantVerdict(t, createdErr, "", errs.VerdictAck)
	for _, p := range r.sender.Sent() {
		if p.UserID == silent {
			t.Fatalf("sent %+v to a member with decision requests off", p)
		}
	}
	if sent := r.sender.Sent(); len(sent) != len(w.voters())-1+len(w.members)-2 {
		t.Fatalf("sent %d pushes, want every other voter's result and every other member's proposal", len(sent))
	}
}

func TestNotify_Result_AVoterOfAFilledProposalGetsOnlyTheResultAndAnIdleMemberNothing(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.resultWorldOf(t, 4, 1)
	deps := module.Deps{Pool: r.pool, UoW: r.uow, IDs: r.ids, Clock: r.clock}
	m := notify.New(
		deps,
		notify.WithSender(r.sender),
		notify.WithCabals(w.ports.cabals),
		notify.WithUsers(w.ports.names),
		notify.WithBallots(
			w.ports.ballots,
		),
		notify.WithProposalHeads(w.ports.heads),
		notify.WithAssets(fixtureCatalog()),
	)

	r.dispatchTo(t, m, tradeActor, r.confirmedEvent(t, w, buyKind, metaTokens, metaUSDC))

	want := make([]string, 0, len(w.voters()))
	for _, user := range w.voters() {
		want = append(want, user.String()+"/"+resultPush)
	}
	r.wantKinds(t, want...)
}

func TestNotify_Result_AnEligibleMemberWhoHasNotVotedGetsTheProposalAndNoResult(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.resultWorldOf(t, 4, 1)
	deps := module.Deps{Pool: r.pool, UoW: r.uow, IDs: r.ids, Clock: r.clock}
	m := notify.New(
		deps,
		notify.WithSender(r.sender),
		notify.WithCabals(w.ports.cabals),
		notify.WithUsers(w.ports.names),
		notify.WithBallots(
			w.ports.ballots,
		),
		notify.WithProposalHeads(w.ports.heads),
		notify.WithAssets(fixtureCatalog()),
	)

	r.dispatchTo(t, m, userActor(w.refs.proposer), r.createdEvent(t, w.refs, buyKind, "AAPLx"))
	r.dispatchTo(t, m, tradeActor, r.confirmedEvent(t, w, buyKind, metaTokens, metaUSDC))

	var idle []apns.Push
	for _, p := range r.sender.Sent() {
		if p.UserID == w.idle {
			idle = append(idle, p)
		}
	}
	if len(idle) != 1 || idle[0].Category != voteCategory || idle[0].Data["kind"] != createdPush {
		t.Fatalf("the idle member got %+v, want the proposal with category %s and no result", idle, voteCategory)
	}
}

func TestNotify_Result_ReachesTheVotersOfARealCabalByDefault(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	dana := testkit.SeedUser(t, r.pool, testkit.UserOpts{Handle: "dana"})
	c := testkit.NewCabal(t, r.pool, testkit.WithCreator(dana.ID), testkit.WithMembers(4), testkit.WithName(cabalName))
	members := make([]ids.UserID, len(c.Members))
	for i, member := range c.Members {
		members[i] = member.ID
		r.device(t, member.ID, token(byte('a'+i)))
	}
	insertRealAsset(t, r, marketfake.AAPLx())
	refs := proposalRefs{cabal: c.ID, proposal: r.ids.NewV7(), proposer: dana.ID}
	r.insertProposal(t, refs, "passed", "", dbBallot{members[0], "yes"}, dbBallot{members[1], "yes"},
		dbBallot{members[2], "no"}, dbBallot{voter: members[3]})
	w := resultWorld{proposalWorld: proposalWorld{cabalWorld: cabalWorld{id: c.ID}, refs: refs}}
	e := r.confirmedEvent(t, w, buyKind, metaTokens, metaUSDC)
	deps := module.Deps{Pool: r.pool, UoW: r.uow, IDs: r.ids, Clock: r.clock}

	d := r.dispatchTo(t, notify.New(deps, notify.WithSender(r.sender)), tradeActor, e)

	r.wantStates(t, d, map[ids.UserID]string{members[0]: "delivered", members[1]: "delivered", members[2]: "delivered"})
	for _, p := range r.sender.Sent() {
		if p.Title != "AAPL passed 2–1" || !strings.HasPrefix(p.Body, "Bought 0.42 sh at $716.52. You voted ") {
			t.Errorf("push %q / %q, want the real tally and fill", p.Title, p.Body)
		}
	}
}

func TestNotify_Result_RenderReadsTheBallotsAndTheHeadAgainAndReturnsTheirFailures(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.resultWorldOf(t, 4, 1)
	voter := w.members[0]
	renders := map[string]func(app.ResultPorts) error{
		"filled": func(p app.ResultPorts) error {
			_, err := app.ResultFilled{
				ResultPorts: p,
			}.Render(
				t.Context(),
				r.confirmedEvent(t, w, buyKind, metaTokens, metaUSDC),
				voter,
			)
			return err
		},
		"fill failed": func(p app.ResultPorts) error {
			_, err := app.ResultFillFailed{ResultPorts: p}.Render(t.Context(), r.failedEvent(t, w, buyKind), voter)
			return err
		},
		"failed": func(p app.ResultPorts) error {
			_, err := app.ResultFailed{
				ResultPorts: p,
			}.Render(
				t.Context(),
				w.closedEvent(events.TypeProposalFailed).(events.ProposalFailed),
				voter,
			)
			return err
		},
	}
	for name, render := range renders {
		for _, op := range []string{"Ballots", "ProposalHead"} {
			if op == "ProposalHead" && name != "failed" {
				continue
			}
			down := errs.New(errs.CodeDBUnavailable, "test."+op)
			ports := proposalPorts{
				cabals: w.ports.cabals, assets: w.ports.assets, ballots: &ballotBox{}, heads: &headBox{},
			}
			ports.fail(op, down)

			if err := render(ports.results()); !errors.Is(err, down) {
				t.Errorf("%s: Render with %s down = %v, want %v", name, op, err, down)
			}
		}
	}
}
