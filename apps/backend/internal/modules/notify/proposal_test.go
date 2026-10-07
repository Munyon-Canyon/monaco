package notify_test

import (
	"context"
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
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
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
	buyKind      = "buy"
	sellKind     = "sell"
	createdPush  = "proposal_created"
	passedPush   = "proposal_passed"
	createdTitle = "New proposal in " + cabalName
	passedTitle  = "Proposal passed"
	buyMicros    = 25_000_000
	sellTokens   = 300_000_000
	sellQuote    = 71_250_000
)

func memberNames() []string { return []string{"Dana", "Bea", "Cy", "Eli", "Fay"} }

type ballots struct {
	byProposal map[ids.ProposalID][]ids.UserID
	err        error
}

func (b *ballots) cast(proposal uuid.UUID, voters ...ids.UserID) *ballots {
	if b.byProposal == nil {
		b.byProposal = map[ids.ProposalID][]ids.UserID{}
	}
	b.byProposal[ids.ProposalIDFrom(proposal)] = voters
	return b
}

func (b *ballots) Voters(_ context.Context, id ids.ProposalID) ([]ids.UserID, error) {
	return b.byProposal[id], b.err
}

type proposalRefs struct {
	cabal    ids.CabalID
	proposal uuid.UUID
	proposer ids.UserID
}

type proposalPorts struct {
	cabals *fakes.Cabal
	names  *fakes.Identity
	voted  *ballots
	assets *marketfake.CatalogFake
}

func (p proposalPorts) fail(op string, err error) {
	switch op {
	case "Members", "Cabal":
		p.cabals.Fail(op, err)
	case "UsersByID":
		p.names.Fail(op, err)
	case "Voters":
		p.voted.err = err
	case "AssetBySymbol":
		p.assets.Fail(op, err)
	}
}

func goldenProposal(t *testing.T) (proposalRefs, proposalPorts) {
	t.Helper()
	created := goldenEvent(t, events.TypeProposalCreated).(events.ProposalCreated)
	refs := proposalRefs{
		cabal: ids.CabalIDFrom(
			created.CabalID,
		),
		proposal: created.ProposalID,
		proposer: ids.UserIDFrom(created.ProposerID),
	}
	seed := fakes.CabalSeed{View: cabal.View{ID: refs.cabal, Name: cabalName}}
	member := fakes.CabalMember{CabalID: refs.cabal, Member: cabal.MemberView{UserID: refs.proposer}}
	return refs, proposalPorts{
		cabals: fakes.NewCabal([]fakes.CabalSeed{seed}, []fakes.CabalMember{member}),
		names:  fakes.NewIdentity([]identity.UserCard{{ID: refs.proposer, DisplayName: memberNames()[0]}}, nil),
		voted:  (&ballots{}).cast(refs.proposal, refs.proposer),
		assets: fixtureCatalog(),
	}
}

type proposalWorld struct {
	cabalWorld
	refs   proposalRefs
	ports  proposalPorts
	tokens map[ids.UserID]string
}

func (r *pushRig) proposalWorldOf(t *testing.T, members int) proposalWorld {
	t.Helper()
	w := proposalWorld{cabalWorld: r.cabalOf(t, members), tokens: map[ids.UserID]string{}}
	w.refs = proposalRefs{cabal: w.id, proposal: r.ids.NewV7(), proposer: w.members[0]}
	cards := make([]identity.UserCard, len(w.members))
	for i, member := range w.members {
		cards[i] = identity.UserCard{ID: member, DisplayName: memberNames()[i]}
		w.tokens[member] = token(byte('a' + i))
	}
	w.ports = proposalPorts{
		cabals: w.cabals, names: fakes.NewIdentity(cards, nil), voted: &ballots{}, assets: fixtureCatalog(),
	}
	return w
}

type proposalLeg struct{ action, symbol string }

type boundKind struct {
	recipients func(ctx context.Context, e events.Event) ([]ids.UserID, error)
	render     func(ctx context.Context, e events.Event) (app.Message, error)
	handle     func(t *testing.T, r *pushRig, d bus.Delivery, e events.Event) error
}

func boundTo[E events.Event](k app.Kind[E]) boundKind {
	return boundKind{
		recipients: func(ctx context.Context, e events.Event) ([]ids.UserID, error) {
			return k.Recipients(ctx, e.(E))
		},
		render: func(ctx context.Context, e events.Event) (app.Message, error) {
			return k.Render(ctx, e.(E), ids.UserID{})
		},
		handle: func(t *testing.T, r *pushRig, d bus.Delivery, e events.Event) error {
			t.Helper()
			return handleKinds(t, r, r.sender, d, e.(E), k)
		},
	}
}

type proposalPush struct {
	action, kind, title, body, unnamedBody string
	reads                                  []string

	event func(t *testing.T, refs proposalRefs, leg proposalLeg) events.Event
	bind  func(p proposalPorts) boundKind
}

type proposalRun struct {
	delivery bus.Delivery
	proposal uuid.UUID
	err      error
}

func (k proposalPush) run(t *testing.T, r *pushRig, w proposalWorld, actor string, leg proposalLeg) proposalRun {
	t.Helper()
	e := k.event(t, w.refs, leg)
	d := r.emit(t, actor, e)
	return proposalRun{delivery: d, proposal: e.AggregateID(), err: k.bind(w.ports).handle(t, r, d, e)}
}

func createdAs(action, body, unnamedBody string) proposalPush {
	return proposalPush{
		action: action, kind: createdPush, title: createdTitle, body: body, unnamedBody: unnamedBody,
		reads: []string{"Members", "Cabal", "UsersByID", "AssetBySymbol"},
		event: func(t *testing.T, refs proposalRefs, leg proposalLeg) events.Event {
			t.Helper()
			e := goldenEvent(t, events.TypeProposalCreated).(events.ProposalCreated)
			e.ProposalID, e.CabalID, e.ProposerID = refs.proposal, refs.cabal.UUID(), refs.proposer.UUID()
			e.Kind, e.Symbol = leg.action, leg.symbol
			if leg.action == sellKind {
				e.USDCMicros, e.TokenAmount, e.QuoteOutAmount = money.Micros{}, sellTokens, sellQuote
			}
			return e
		},
		bind: func(p proposalPorts) boundKind {
			return boundTo[events.ProposalCreated](
				app.ProposalCreated{Cabals: p.cabals, Users: p.names, Assets: p.assets},
			)
		},
	}
}

func passedAs(action, body, unnamedBody string) proposalPush {
	return proposalPush{
		action: action, kind: passedPush, title: passedTitle, body: body, unnamedBody: unnamedBody,
		reads: []string{"Voters", "Members", "AssetBySymbol"},
		event: func(t *testing.T, refs proposalRefs, leg proposalLeg) events.Event {
			t.Helper()
			e := goldenEvent(t, events.TypeProposalPassed).(events.ProposalPassed)
			e.ProposalID, e.CabalID, e.ProposerID = refs.proposal, refs.cabal.UUID(), refs.proposer.UUID()
			e.Kind, e.Symbol = leg.action, leg.symbol
			if leg.action == buyKind {
				e.USDCMicros, e.TokenAmount = money.MicrosFromUint64(buyMicros), 0
			}
			return e
		},
		bind: func(p proposalPorts) boundKind {
			return boundTo[events.ProposalPassed](
				app.ProposalPassed{Cabals: p.cabals, Voters: p.voted, Assets: p.assets},
			)
		},
	}
}

func proposalPushes() []proposalPush {
	return []proposalPush{
		createdAs(buyKind, "Dana wants to buy $25.00 of Apple. Vote now.",
			"Dana wants to buy $25.00 of a stock. Vote now."),
		createdAs(sellKind, "Dana wants to sell about $71.25 of Apple. Vote now.",
			"Dana wants to sell about $71.25 of a stock. Vote now."),
		passedAs(buyKind, "Your cabal's buy of $25.00 of Apple passed. Buying now.",
			"Your cabal's buy of $25.00 of a stock passed. Buying now."),
		passedAs(sellKind, "Your cabal's sell of Apple passed. Selling now.",
			"Your cabal's sell of a stock passed. Selling now."),
	}
}

func proposalPushesWhere(keep func(proposalPush) bool) []proposalPush {
	return slices.DeleteFunc(proposalPushes(), func(k proposalPush) bool { return !keep(k) })
}

func userActor(user ids.UserID) string { return "user:" + user.String() }

func (r *pushRig) wantProposalPushed(
	t *testing.T, w proposalWorld, k proposalPush, run proposalRun, recipients ...ids.UserID,
) {
	t.Helper()
	byToken := func(a, b apns.Push) int { return strings.Compare(a.Token, b.Token) }
	want := make([]apns.Push, 0, len(recipients))
	states := map[ids.UserID]string{}
	for _, user := range recipients {
		states[user] = "delivered"
		want = append(want, apns.Push{
			UserID: user, Token: w.tokens[user], Environment: apns.Sandbox,
			CollapseID: "proposal-" + run.proposal.String(), Title: k.title, Body: k.body,
			Data: map[string]string{"kind": k.kind, "cabal_id": w.id.String(), "proposal_id": run.proposal.String()},
		})
	}
	slices.SortFunc(want, byToken)
	sent := r.sender.Sent()
	slices.SortFunc(sent, byToken)
	if !reflect.DeepEqual(sent, want) {
		t.Fatalf("sent %+v, want one push per recipient and no one else: %+v", sent, want)
	}
	r.wantStates(t, run.delivery, states)
	if len(recipients) > 1 {
		r.wantCount(t, "rows in one broadcast of the recipients", len(recipients), `SELECT count(*) FROM notifications n
			JOIN notification_broadcasts b ON b.id = n.broadcast_id
			WHERE b.source_event_id = $1 AND b.recipient_count = $2 AND b.kind = $3`,
			run.delivery.EventID.UUID(), len(recipients), k.kind)
	}
	r.wantSentEvents(t, run.delivery, len(recipients))
	r.wantRecorded(t, run.delivery, 1)
}

func TestNotify_ProposalCreated_MembersExceptProposer(t *testing.T) {
	t.Parallel()
	for _, k := range proposalPushesWhere(func(k proposalPush) bool { return k.kind == createdPush }) {
		t.Run(k.action, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			w := r.proposalWorldOf(t, 3)

			run := k.run(t, r, w, userActor(w.refs.proposer), proposalLeg{k.action, "AAPLx"})

			wantVerdict(t, run.err, "", errs.VerdictAck)
			r.wantProposalPushed(t, w, k, run, w.members[1:]...)
		})
	}
}

func TestNotify_ProposalPassed_VotersOnly(t *testing.T) {
	t.Parallel()
	for _, k := range proposalPushesWhere(func(k proposalPush) bool { return k.kind == passedPush }) {
		t.Run(k.action, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			w := r.proposalWorldOf(t, 4)
			yes, deciding, no := w.members[0], w.members[1], w.members[2]
			left := r.user(t, "active")
			r.device(t, left, token('y'))
			w.ports.voted.cast(w.refs.proposal, left, no, yes, deciding)

			run := k.run(t, r, w, userActor(deciding), proposalLeg{k.action, "AAPLx"})

			wantVerdict(t, run.err, "", errs.VerdictAck)
			r.wantProposalPushed(t, w, k, run, yes, no)
		})
	}
}

func TestNotify_ProposalRecipients_FollowThePortOrderAndTheBallots(t *testing.T) {
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
	proposal := g.NewV7()
	created := goldenEvent(t, events.TypeProposalCreated).(events.ProposalCreated)
	passed := goldenEvent(t, events.TypeProposalPassed).(events.ProposalPassed)
	created.CabalID, passed.CabalID, passed.ProposalID = cabalID.UUID(), cabalID.UUID(), proposal
	voted := (&ballots{}).cast(proposal, first, left, third, second)

	createdTo, createdErr := app.ProposalCreated{Cabals: cabals}.Recipients(t.Context(), created)
	passedTo, passedErr := app.ProposalPassed{Cabals: cabals, Voters: voted}.Recipients(t.Context(), passed)

	if createdErr != nil || passedErr != nil {
		t.Fatalf("Recipients = %v and %v, want no error", createdErr, passedErr)
	}
	if want := []ids.UserID{third, second, first, idle}; !slices.Equal(createdTo, want) {
		t.Errorf("created to %v, want every member in port order %v", createdTo, want)
	}
	if want := []ids.UserID{third, second, first}; !slices.Equal(passedTo, want) {
		t.Errorf("passed to %v, want the voters who are still members, in port order, %v", passedTo, want)
	}
}

func TestNotify_ProposalCreated_NamesTheProposerOrSomeone(t *testing.T) {
	t.Parallel()
	refs, ports := goldenProposal(t)
	created := goldenEvent(t, events.TypeProposalCreated).(events.ProposalCreated)
	named := identity.UserCard{DisplayName: "Dana"}
	for name, tc := range map[string]struct {
		cards map[ids.UserID]identity.UserCard
		want  string
	}{
		"a named member":        {map[ids.UserID]identity.UserCard{refs.proposer: named}, "Dana"},
		"a member with no name": {map[ids.UserID]identity.UserCard{refs.proposer: {}}, "Someone"},
		"a user the port lacks": {nil, "Someone"},
		"a deleted user whose card still has a name": {
			map[ids.UserID]identity.UserCard{refs.proposer: {DisplayName: "Dana", Deleted: true}}, "Someone",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			kind := app.ProposalCreated{Cabals: ports.cabals, Users: users{cards: tc.cards}, Assets: ports.assets}

			msg, err := kind.Render(t.Context(), created, ids.UserID{})

			if want := tc.want + " wants to buy $25.00 of Apple. Vote now."; err != nil || msg.Body != want {
				t.Fatalf("Render = %q, %v, want %q", msg.Body, err, want)
			}
		})
	}
}

func TestNotify_Proposal_BothKindsShareOneCollapseID(t *testing.T) {
	t.Parallel()
	refs, ports := goldenProposal(t)
	collapse := make([]string, 0, 2)
	for _, k := range proposalPushesWhere(func(k proposalPush) bool { return k.action == buyKind }) {
		msg, err := k.bind(ports).render(t.Context(), k.event(t, refs, proposalLeg{k.action, "AAPLx"}))
		if err != nil {
			t.Fatalf("%s: Render = %v", k.kind, err)
		}
		collapse = append(collapse, msg.CollapseID)
	}
	if want := "proposal-" + refs.proposal.String(); !slices.Equal(collapse, []string{want, want}) {
		t.Fatalf("collapse ids %v, want %s for both kinds so passed replaces created on the phone", collapse, want)
	}
}

func TestNotify_Proposal_AnUnknownSymbolReadsAsAStock(t *testing.T) {
	t.Parallel()
	refs, ports := goldenProposal(t)
	for _, k := range proposalPushes() {
		msg, err := k.bind(ports).render(t.Context(), k.event(t, refs, proposalLeg{k.action, "ZZZZx"}))

		if err != nil || msg.Title != k.title || msg.Body != k.unnamedBody {
			t.Errorf("%s/%s: Render = %q / %q, %v, want %q / %q", k.kind, k.action, msg.Title, msg.Body, err,
				k.title, k.unnamedBody)
		}
	}
}

func TestNotify_Proposal_PortFailuresReturnAsIsAndNak(t *testing.T) {
	t.Parallel()
	for _, k := range proposalPushesWhere(func(k proposalPush) bool { return k.action == buyKind }) {
		for _, op := range k.reads {
			t.Run(k.kind+"/"+op, func(t *testing.T) {
				t.Parallel()
				refs, ports := goldenProposal(t)
				down := errs.New(errs.CodeDBUnavailable, "test."+op)
				ports.fail(op, down)
				bound, e := k.bind(ports), k.event(t, refs, proposalLeg{k.action, "AAPLx"})

				_, err := bound.recipients(t.Context(), e)
				if err == nil {
					_, err = bound.render(t.Context(), e)
				}

				if !errors.Is(err, down) {
					t.Fatalf("Recipients and Render = %v, want the port error itself, %v", err, down)
				}
				wantVerdict(t, err, errs.CodeDBUnavailable, errs.VerdictNak)
			})
		}
	}
}

func TestNotify_Proposal_RefusesAKindItCannotPhraseBeforeReadingAnyPort(t *testing.T) {
	t.Parallel()
	for _, k := range proposalPushesWhere(func(k proposalPush) bool { return k.action == buyKind }) {
		t.Run(k.kind, func(t *testing.T) {
			t.Parallel()
			refs, ports := goldenProposal(t)
			for _, op := range []string{"Members", "Cabal", "UsersByID", "Voters", "AssetBySymbol"} {
				ports.fail(op, errs.New(errs.CodeDBUnavailable, "test."+op))
			}

			_, err := k.bind(ports).render(t.Context(), k.event(t, refs, proposalLeg{"swap", "AAPLx"}))

			wantVerdict(t, err, errs.CodeInvalidInput, errs.VerdictTerm)
		})
	}
}

func (r *pushRig) insertProposal(t *testing.T, refs proposalRefs, voters ...ids.UserID) {
	t.Helper()
	r.exec(t, `INSERT INTO proposals (id, cabal_id, proposer_id, kind, symbol, mint, usdc_micros, quote_out_amount,
		threshold, status, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, 'buy', 'AAPLx', 'mint', $4, 105000000, 'majority', 'passed', $5, $6, $6)`,
		refs.proposal, refs.cabal.UUID(), refs.proposer.UUID(), buyMicros, r.clock.Now().Add(time.Hour), r.clock.Now())
	for _, voter := range voters {
		r.exec(t, `INSERT INTO votes (proposal_id, voter_id, choice, cast_at) VALUES ($1, $2, 'yes', $3)`,
			refs.proposal, voter.UUID(), r.clock.Now())
	}
}

func (r *pushRig) wantModulePushedProposal(
	t *testing.T, d bus.Delivery, k proposalPush, body string, recipients []ids.UserID,
) {
	t.Helper()
	states := map[ids.UserID]string{}
	for _, user := range recipients {
		states[user] = "delivered"
	}
	r.wantStates(t, d, states)
	sent := r.sender.Sent()
	if len(sent) != len(recipients) {
		t.Fatalf("sent %d pushes, want one per recipient of %d", len(sent), len(recipients))
	}
	for _, p := range sent {
		if p.Title != k.title || p.Body != body {
			t.Errorf("push %q / %q, want %q / %q", p.Title, p.Body, k.title, body)
		}
	}
	r.wantRecorded(t, d, 1)
}

func TestNotify_Proposal_ReachesTheRealMembersAndVotersAndNamesRealThingsByDefault(t *testing.T) {
	t.Parallel()
	for _, k := range proposalPushesWhere(func(k proposalPush) bool { return k.action == buyKind }) {
		t.Run(k.kind, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			dana := testkit.SeedUser(t, r.pool, testkit.UserOpts{Handle: "dana"})
			c := testkit.NewCabal(t, r.pool, testkit.WithCreator(dana.ID), testkit.WithMembers(3),
				testkit.WithName(cabalName))
			members := make([]ids.UserID, len(c.Members))
			for i, member := range c.Members {
				members[i] = member.ID
				r.device(t, member.ID, token(byte('a'+i)))
			}
			r.device(t, r.user(t, "active"), token('z'))
			insertRealAsset(t, r, marketfake.AAPLx())
			refs := proposalRefs{cabal: c.ID, proposal: r.ids.NewV7(), proposer: dana.ID}
			r.insertProposal(t, refs, members[1], members[2])
			deps := module.Deps{Pool: r.pool, UoW: r.uow, IDs: r.ids, Clock: r.clock}
			wantTo, actor := members[1:], userActor(dana.ID)
			if k.kind == passedPush {
				wantTo, actor = members[2:], userActor(members[1])
			}
			m := notify.New(deps, notify.WithSender(r.sender))
			e := k.event(t, refs, proposalLeg{k.action, "AAPLx"})

			d := r.dispatchTo(t, m, actor, e)

			r.wantModulePushedProposal(t, d, k, strings.Replace(k.body, "Dana", "dana", 1), wantTo)
		})
	}
}

func TestNotify_Proposal_ReachesTheMembersAndVotersAndNamesTheAssetOfThePortsItIsGiven(t *testing.T) {
	t.Parallel()
	for _, k := range proposalPushesWhere(func(k proposalPush) bool { return k.action == buyKind }) {
		t.Run(k.kind, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			w := r.proposalWorldOf(t, 3)
			w.ports.voted.cast(w.refs.proposal, w.members[1:]...)
			pear := marketfake.AAPLx()
			pear.DisplayName = "Pear"
			deps := module.Deps{Pool: r.pool, UoW: r.uow, IDs: r.ids, Clock: r.clock}
			m := notify.New(deps, notify.WithSender(r.sender), notify.WithCabals(w.ports.cabals),
				notify.WithUsers(w.ports.names), notify.WithVoters(w.ports.voted),
				notify.WithAssets(marketfake.NewCatalog(pear)))
			wantTo := w.members
			if k.kind == passedPush {
				wantTo = w.members[1:]
			}

			d := r.dispatchTo(t, m, opsActor, k.event(t, w.refs, proposalLeg{k.action, pear.Symbol}))

			r.wantModulePushedProposal(t, d, k, strings.Replace(k.body, "Apple", "Pear", 1), wantTo)
		})
	}
}
