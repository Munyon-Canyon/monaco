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
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
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
	resultPush   = "proposal_result"
	thesis       = "Small add before earnings."
	buyMicros    = 25_000_000
	sellTokens   = 300_000_000
	sellQuote    = 71_250_000
	voteWindow   = 6 * time.Hour
	voteCategory = "VOTE"
)

func memberNames() []string { return []string{"Dana", "Bea", "Cy", "Eli", "Fay", "Gus"} }

func yesBallot(user ids.UserID) governanceport.Ballot { return ballotOf(user, "yes") }

func noBallot(user ids.UserID) governanceport.Ballot { return ballotOf(user, "no") }

func ballotOf(user ids.UserID, choice string) governanceport.Ballot {
	at := time.Date(2026, 3, 1, 11, 0, 0, 0, time.UTC)
	return governanceport.Ballot{VoterID: user, Choice: &choice, CastAt: &at}
}

func eligibleBallot(user ids.UserID) governanceport.Ballot {
	return governanceport.Ballot{VoterID: user}
}

type ballotBox struct {
	byProposal map[uuid.UUID][]governanceport.Ballot
	err        error
}

func (b *ballotBox) set(proposal uuid.UUID, ballots ...governanceport.Ballot) *ballotBox {
	if b.byProposal == nil {
		b.byProposal = map[uuid.UUID][]governanceport.Ballot{}
	}
	b.byProposal[proposal] = ballots
	return b
}

func (b *ballotBox) Ballots(_ context.Context, id ids.ProposalID) ([]governanceport.Ballot, error) {
	return b.byProposal[id.UUID()], b.err
}

type headBox struct {
	byProposal map[uuid.UUID]governanceport.ProposalHead
	err        error
}

func (h *headBox) set(proposal uuid.UUID, head governanceport.ProposalHead) *headBox {
	if h.byProposal == nil {
		h.byProposal = map[uuid.UUID]governanceport.ProposalHead{}
	}
	h.byProposal[proposal] = head
	return h
}

func (h *headBox) ProposalHead(_ context.Context, id ids.ProposalID) (governanceport.ProposalHead, error) {
	return h.byProposal[id.UUID()], h.err
}

type proposalRefs struct {
	cabal    ids.CabalID
	proposal uuid.UUID
	proposer ids.UserID
}

type proposalPorts struct {
	cabals  *fakes.Cabal
	names   *fakes.Identity
	ballots *ballotBox
	heads   *headBox
	assets  *marketfake.CatalogFake
}

func (p proposalPorts) fail(op string, err error) {
	switch op {
	case "Members", "Cabal":
		p.cabals.Fail(op, err)
	case "UsersByID":
		p.names.Fail(op, err)
	case "Ballots":
		p.ballots.err = err
	case "ProposalHead":
		p.heads.err = err
	case "AssetBySymbol":
		p.assets.Fail(op, err)
	}
}

func (p proposalPorts) created(r *pushRig) app.ProposalCreated {
	return app.ProposalCreated{
		Cabals: p.cabals, Users: p.names, Assets: p.assets, Ballots: p.ballots, Heads: p.heads, Clock: r.clock,
	}
}

func (p proposalPorts) results() app.ResultPorts {
	return app.ResultPorts{Cabals: p.cabals, Assets: p.assets, Ballots: p.ballots, Heads: p.heads}
}

func goldenProposal(t *testing.T) (proposalRefs, proposalPorts) {
	t.Helper()
	created := goldenEvent(t, events.TypeProposalCreated).(events.ProposalCreated)
	refs := proposalRefs{
		cabal: ids.CabalIDFrom(created.CabalID), proposal: created.ProposalID,
		proposer: ids.UserIDFrom(created.ProposerID),
	}
	seed := fakes.CabalSeed{View: cabal.View{ID: refs.cabal, Name: cabalName}}
	member := fakes.CabalMember{CabalID: refs.cabal, Member: cabal.MemberView{UserID: refs.proposer}}
	return refs, proposalPorts{
		cabals:  fakes.NewCabal([]fakes.CabalSeed{seed}, []fakes.CabalMember{member}),
		names:   fakes.NewIdentity([]identity.UserCard{{ID: refs.proposer, DisplayName: memberNames()[0]}}, nil),
		ballots: (&ballotBox{}).set(refs.proposal, eligibleBallot(refs.proposer)),
		heads: (&headBox{}).set(
			refs.proposal,
			governanceport.ProposalHead{Kind: buyKind, Symbol: "AAPLx", Thesis: thesis},
		),
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
		cabals:  w.cabals,
		names:   fakes.NewIdentity(cards, nil),
		ballots: &ballotBox{},
		heads: (&headBox{}).set(
			w.refs.proposal,
			governanceport.ProposalHead{Kind: buyKind, Symbol: "AAPLx", Thesis: thesis},
		),
		assets: fixtureCatalog(),
	}
	return w
}

func (r *pushRig) createdEvent(t *testing.T, refs proposalRefs, action, symbol string) events.ProposalCreated {
	t.Helper()
	e := goldenEvent(t, events.TypeProposalCreated).(events.ProposalCreated)
	e.ProposalID, e.CabalID, e.ProposerID = refs.proposal, refs.cabal.UUID(), refs.proposer.UUID()
	e.Kind, e.Symbol, e.ExpiresAt = action, symbol, r.clock.Now().Add(voteWindow)
	if action == sellKind {
		e.USDCMicros, e.TokenAmount, e.QuoteOutAmount = money.Micros{}, sellTokens, sellQuote
	}
	return e
}

func userActor(user ids.UserID) string { return "user:" + user.String() }

func TestNotify_ProposalCreated_MembersExceptProposer(t *testing.T) {
	t.Parallel()
	for action, want := range map[string]struct{ title, body string }{
		buyKind:  {"Dana proposed Buy AAPL $25", "“Small add before earnings.” The vote closes in 6h."},
		sellKind: {"Dana proposed Sell AAPL $71.25", "“Small add before earnings.” The vote closes in 6h."},
	} {
		t.Run(action, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			w := r.proposalWorldOf(t, 4)
			frozen := w.members[:3]
			w.ports.ballots.set(
				w.refs.proposal,
				eligibleBallot(frozen[0]),
				eligibleBallot(frozen[1]),
				yesBallot(frozen[2]),
			)
			w.ports.heads.set(
				w.refs.proposal,
				governanceport.ProposalHead{Kind: action, Symbol: "AAPLx", Thesis: thesis},
			)
			e := r.createdEvent(t, w.refs, action, "AAPLx")
			d := r.emit(t, userActor(w.refs.proposer), e)

			err := handleKinds(t, r, r.sender, d, e, w.ports.created(r))

			wantVerdict(t, err, "", errs.VerdictAck)
			byToken := func(a, b apns.Push) int { return strings.Compare(a.Token, b.Token) }
			wantSent := make([]apns.Push, 0, len(w.members)-1)
			states := map[ids.UserID]string{}
			for _, user := range w.members[1:] {
				states[user] = "delivered"
				p := inCabal(apns.Push{
					UserID: user, Token: w.tokens[user], Environment: apns.Sandbox,
					CollapseID: "proposal-" + w.refs.proposal.String(), Title: want.title, Body: want.body,
					Data: map[string]string{
						"kind": createdPush, "cabal_id": w.id.String(), "proposal_id": w.refs.proposal.String(),
					},
				}, w.id, wantSender{"person", w.refs.proposer.String(), memberNames()[0]})
				if slices.Contains(frozen, user) {
					p.Category = voteCategory
				}
				wantSent = append(wantSent, p)
			}
			slices.SortFunc(wantSent, byToken)
			sent := r.sender.Sent()
			slices.SortFunc(sent, byToken)
			if !reflect.DeepEqual(sent, wantSent) {
				t.Fatalf("sent %+v, want the three other members, VOTE only for the frozen set: %+v", sent, wantSent)
			}
			r.wantStates(t, d, states)
			r.wantRecorded(t, d, 1)
		})
	}
}

func TestNotify_ProposalCreated_RecipientsFollowThePortOrderAndSkipRules(t *testing.T) {
	t.Parallel()
	g := testkit.NewIDs(1)
	cabalID := ids.CabalIDFrom(g.NewV7())
	first, second, third, idle := ids.NewUserID(g), ids.NewUserID(g), ids.NewUserID(g), ids.NewUserID(g)
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
	created := goldenEvent(t, events.TypeProposalCreated).(events.ProposalCreated)
	created.CabalID = cabalID.UUID()

	createdTo, createdErr := app.ProposalCreated{Cabals: cabals}.Recipients(t.Context(), created)
	created.Kind = "rule"
	ruleTo, ruleErr := app.ProposalCreated{Cabals: cabals}.Recipients(t.Context(), created)

	if createdErr != nil || ruleErr != nil {
		t.Fatalf("Recipients = %v and %v, want no error", createdErr, ruleErr)
	}
	if want := []ids.UserID{third, second, first, idle}; !slices.Equal(createdTo, want) {
		t.Errorf("created to %v, want every member in port order %v", createdTo, want)
	}
	if len(ruleTo) != 0 {
		t.Errorf("a rule proposal went to %v, want nobody", ruleTo)
	}
}

func TestNotify_ProposalCreated_NamesTheProposerOrSomeone(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		card *identity.UserCard
		want string
	}{
		"a named member":                             {&identity.UserCard{DisplayName: "Dana"}, "Dana"},
		"a member with no name":                      {&identity.UserCard{}, "Someone"},
		"a user the port lacks":                      {nil, "Someone"},
		"a deleted user whose card still has a name": {&identity.UserCard{DisplayName: "Dana", Deleted: true}, "Someone"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			refs, ports := goldenProposal(t)
			cards := map[ids.UserID]identity.UserCard{}
			if tc.card != nil {
				cards[refs.proposer] = *tc.card
			}
			kind := ports.created(r)
			kind.Users = users{cards: cards}

			msg, err := kind.Render(t.Context(), r.createdEvent(t, refs, buyKind, "AAPLx"), refs.proposer)

			if want := tc.want + " proposed Buy AAPL $25"; err != nil || msg.Title != want {
				t.Fatalf("Render = %q, %v, want %q", msg.Title, err, want)
			}
		})
	}
}

func TestNotify_ProposalCreated_ReadsTheThesisAndTheCloseTime(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("é", 121)
	for name, tc := range map[string]struct {
		thesis string
		left   time.Duration
		want   string
	}{
		"a thesis":                       {thesis, voteWindow, "“Small add before earnings.” The vote closes in 6h."},
		"no thesis":                      {"", voteWindow, "The vote closes in 6h."},
		"a blank thesis":                 {"  \n", voteWindow, "The vote closes in 6h."},
		"a thesis of exactly 120 runes":  {long[:240], voteWindow, "“" + long[:240] + "” The vote closes in 6h."},
		"a thesis over 120 runes":        {long, voteWindow, "“" + long[:240] + "…” The vote closes in 6h."},
		"minutes under an hour":          {"", 45 * time.Minute, "The vote closes in 45m."},
		"the last minute":                {"", 20 * time.Second, "The vote closes in 0m."},
		"hours floor under 48":           {"", 47*time.Hour + 20*time.Minute, "The vote closes in 47h."},
		"a few ms short of the window":   {"", voteWindow - 40*time.Millisecond, "The vote closes in 6h."},
		"days from 48 hours":             {"", 48 * time.Hour, "The vote closes in 2d."},
		"days floor":                     {"", 71 * time.Hour, "The vote closes in 2d."},
		"an hour is hours, not minutes ": {"", time.Hour, "The vote closes in 1h."},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			refs, ports := goldenProposal(t)
			ports.heads.set(
				refs.proposal,
				governanceport.ProposalHead{Kind: buyKind, Symbol: "AAPLx", Thesis: tc.thesis},
			)
			e := r.createdEvent(t, refs, buyKind, "AAPLx")
			e.ExpiresAt = r.clock.Now().Add(tc.left)

			msg, err := ports.created(r).Render(t.Context(), e, refs.proposer)

			if err != nil || msg.Body != tc.want {
				t.Fatalf("Render = %q, %v, want %q", msg.Body, err, tc.want)
			}
		})
	}
}

func TestNotify_Proposal_AnUnknownSymbolReadsAsAStock(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	refs, ports := goldenProposal(t)

	msg, err := ports.created(r).Render(t.Context(), r.createdEvent(t, refs, buyKind, "ZZZZx"), refs.proposer)

	if want := "Dana proposed Buy a stock $25"; err != nil || msg.Title != want {
		t.Fatalf("Render = %q, %v, want %q", msg.Title, err, want)
	}
	if want := "proposal-" + refs.proposal.String(); msg.CollapseID != want {
		t.Fatalf("collapse id %q, want %q so the result replaces the proposal on the phone", msg.CollapseID, want)
	}
}

func TestNotify_Proposal_PortFailuresReturnAsIsAndNak(t *testing.T) {
	t.Parallel()
	for _, op := range []string{"Members", "Cabal", "UsersByID", "AssetBySymbol", "Ballots", "ProposalHead"} {
		t.Run(op, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			refs, ports := goldenProposal(t)
			down := errs.New(errs.CodeDBUnavailable, "test."+op)
			ports.fail(op, down)
			kind, e := ports.created(r), r.createdEvent(t, refs, buyKind, "AAPLx")

			_, err := kind.Recipients(t.Context(), e)
			if err == nil {
				_, err = kind.Render(t.Context(), e, refs.proposer)
			}

			if !errors.Is(err, down) {
				t.Fatalf("Recipients and Render = %v, want the port error itself, %v", err, down)
			}
			wantVerdict(t, err, errs.CodeDBUnavailable, errs.VerdictNak)
		})
	}
}

func TestNotify_Proposal_RefusesAKindItCannotPhraseBeforeReadingAnyPort(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	refs, ports := goldenProposal(t)
	for _, op := range []string{"Members", "Cabal", "UsersByID", "Ballots", "ProposalHead", "AssetBySymbol"} {
		ports.fail(op, errs.New(errs.CodeDBUnavailable, "test."+op))
	}

	_, err := ports.created(r).Render(t.Context(), r.createdEvent(t, refs, "swap", "AAPLx"), refs.proposer)

	wantVerdict(t, err, errs.CodeInvalidInput, errs.VerdictTerm)
}

type dbBallot struct {
	voter  ids.UserID
	choice string
}

func (r *pushRig) insertProposal(t *testing.T, refs proposalRefs, status, text string, ballots ...dbBallot) {
	t.Helper()
	r.exec(t, `INSERT INTO proposals (id, cabal_id, proposer_id, kind, symbol, mint, usdc_micros, quote_out_amount,
		threshold, status, expires_at, created_at, updated_at, thesis)
		VALUES ($1, $2, $3, 'buy', 'AAPLx', 'mint', $4, 105000000, 'majority', $7, $5, $6, $6, NULLIF($8, ''))`,
		refs.proposal, refs.cabal.UUID(), refs.proposer.UUID(), buyMicros, r.clock.Now().Add(voteWindow),
		r.clock.Now(), status, text)
	for _, b := range ballots {
		r.exec(t, `INSERT INTO proposal_voters (proposal_id, voter_id) VALUES ($1, $2)`, refs.proposal, b.voter.UUID())
		if b.choice != "" {
			r.exec(
				t,
				`INSERT INTO votes (proposal_id, voter_id, choice, cast_at, first_cast_at) VALUES ($1, $2, $3, $4, $4)`,
				refs.proposal,
				b.voter.UUID(),
				b.choice,
				r.clock.Now(),
			)
		}
	}
}

func TestNotify_Proposal_ReachesTheRealMembersAndVotersAndNamesRealThingsByDefault(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	dana := testkit.SeedUser(t, r.pool, testkit.UserOpts{Handle: "dana"})
	c := testkit.NewCabal(t, r.pool, testkit.WithCreator(dana.ID), testkit.WithMembers(3), testkit.WithName(cabalName))
	members := make([]ids.UserID, len(c.Members))
	for i, member := range c.Members {
		members[i] = member.ID
		r.device(t, member.ID, token(byte('a'+i)))
	}
	insertRealAsset(t, r, marketfake.AAPLx())
	refs := proposalRefs{cabal: c.ID, proposal: r.ids.NewV7(), proposer: dana.ID}
	r.insertProposal(t, refs, "open", thesis, dbBallot{voter: members[1]})
	deps := module.Deps{Pool: r.pool, UoW: r.uow, IDs: r.ids, Clock: r.clock}

	d := r.dispatchTo(t, notify.New(deps, notify.WithSender(r.sender)), userActor(dana.ID),
		r.createdEvent(t, refs, buyKind, "AAPLx"))

	r.wantStates(t, d, map[ids.UserID]string{members[1]: "delivered", members[2]: "delivered"})
	categories := map[string]string{}
	for _, p := range r.sender.Sent() {
		if p.Title != "dana proposed Buy AAPL $25" || p.Body != "“"+thesis+"” The vote closes in 6h." {
			t.Errorf("push %q / %q, want the thesis and the close time", p.Title, p.Body)
		}
		categories[p.Token] = p.Category
	}
	if want := map[string]string{token('b'): voteCategory, token('c'): ""}; !reflect.DeepEqual(categories, want) {
		t.Fatalf("categories by token %v, want %v", categories, want)
	}
}

func TestNotify_Proposal_ReachesTheMembersAndVotersAndNamesTheAssetOfThePortsItIsGiven(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.proposalWorldOf(t, 3)
	w.ports.ballots.set(w.refs.proposal, eligibleBallot(w.members[1]), eligibleBallot(w.members[2]))
	pear := marketfake.AAPLx()
	pear.Symbol = "PEARx"
	deps := module.Deps{Pool: r.pool, UoW: r.uow, IDs: r.ids, Clock: r.clock}
	m := notify.New(deps, notify.WithSender(r.sender), notify.WithCabals(w.ports.cabals),
		notify.WithUsers(w.ports.names), notify.WithBallots(w.ports.ballots), notify.WithProposalHeads(w.ports.heads),
		notify.WithAssets(marketfake.NewCatalog(pear)))

	d := r.dispatchTo(t, m, userActor(w.refs.proposer), r.createdEvent(t, w.refs, buyKind, "PEARx"))

	r.wantStates(t, d, map[ids.UserID]string{w.members[1]: "delivered", w.members[2]: "delivered"})
	for _, p := range r.sender.Sent() {
		if p.Title != "Dana proposed Buy PEAR $25" || p.Category != voteCategory {
			t.Errorf("push %q in category %q, want the port's asset and VOTE", p.Title, p.Category)
		}
	}
}
