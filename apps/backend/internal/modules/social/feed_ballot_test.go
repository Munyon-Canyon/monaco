package social_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

type fakeBallots struct {
	calls  atomic.Int32
	viewer atomic.Value
	byUser map[ids.UserID]map[ids.ProposalID]app.ViewerBallot
	err    error
}

func (f *fakeBallots) MyBallots(
	_ context.Context, viewer ids.UserID, proposals []ids.ProposalID,
) (map[ids.ProposalID]app.ViewerBallot, error) {
	f.calls.Add(1)
	f.viewer.Store(viewer)
	if f.err != nil {
		return nil, f.err
	}
	out := map[ids.ProposalID]app.ViewerBallot{}
	for _, id := range proposals {
		if b, ok := f.byUser[viewer][id]; ok {
			out[id] = b
		}
	}
	return out, nil
}

type ballotPage struct {
	r       proposalRig
	routes  adapters.HTTP
	ballots *fakeBallots
	open    uuid.UUID
	closed  uuid.UUID
	second  uuid.UUID
}

func newBallotPage(t *testing.T) ballotPage {
	t.Helper()
	r := newProposalRig(t)
	closedID, secondID := r.gen.NewV7(), r.gen.NewV7()
	p := ballotPage{r: r}
	p.open = r.proposalItem(t, r.proposal, r.cabal)
	p.closed = r.proposalItem(t, closedID, r.cabal)
	p.second = r.proposalItem(t, secondID, r.cabal)
	p.ballots = &fakeBallots{byUser: map[ids.UserID]map[ids.ProposalID]app.ViewerBallot{
		r.bob: {
			ids.ProposalIDFrom(r.proposal): {CanVote: true},
			ids.ProposalIDFrom(closedID):   {Choice: "yes", CanVote: false},
			ids.ProposalIDFrom(secondID):   {Choice: "no", CanVote: true},
		},
	}}
	feedFixture{pool: r.pool, gen: r.gen, clock: r.clock}.item(t, func(it *feed.Item) {
		it.Kind, it.CabalID, it.ActorID = feed.KindTrade, ids.CabalIDFrom(r.cabal), r.bob
		it.Payload = feed.Payload{CabalName: "Alpha", Symbol: "AAPLx", Action: feed.ActionBuy}
	})
	f := feedFixture{pool: r.pool, gen: r.gen, clock: r.clock}
	p.routes = f.routesOptions(social.WithUsers(r.users), social.WithBallots(p.ballots))
	return p
}

func (p ballotPage) read(t *testing.T, viewer ids.UserID) map[uuid.UUID]api.FeedItem {
	t.Helper()
	res, err := p.routes.GetFeed(asUser(t.Context(), viewer), api.GetFeedRequestObject{Params: api.GetFeedParams{}})
	if err != nil {
		t.Fatal(err)
	}
	out := map[uuid.UUID]api.FeedItem{}
	for _, item := range res.(api.GetFeed200JSONResponse).Items {
		out[item.Id] = item
	}
	return out
}

func wantBallot(t *testing.T, item api.FeedItem, ballot string, canVote bool) {
	t.Helper()
	switch {
	case ballot == "" && item.MyBallot != nil,
		ballot != "" && (item.MyBallot == nil || string(*item.MyBallot) != ballot):
		t.Fatalf("my_ballot = %v, want %q", item.MyBallot, ballot)
	case item.CanVote != canVote:
		t.Fatalf("can_vote = %v, want %v", item.CanVote, canVote)
	}
}

func TestGetFeed_aProposalSaysHowTheViewerVotedAndWhetherTheyCanFromOneCallPerPage(t *testing.T) {
	t.Parallel()
	p := newBallotPage(t)
	items := p.read(t, p.r.bob)
	if got := p.ballots.calls.Load(); got != 1 {
		t.Fatalf("MyBallots called %d times for one page, want 1", got)
	}
	if viewer := p.ballots.viewer.Load(); viewer != p.r.bob {
		t.Fatalf("MyBallots viewer = %v, want the caller", viewer)
	}
	wantBallot(t, items[p.open], "", true)
	wantBallot(t, items[p.closed], "yes", false)
	wantBallot(t, items[p.second], "no", true)
	for _, item := range items {
		if item.Kind == "trade" {
			wantBallot(t, item, "", false)
		}
	}
}

func TestGetFeed_anOutsiderCannotVoteAndHasNoBallot(t *testing.T) {
	t.Parallel()
	p := newBallotPage(t)
	seen := 0
	for _, item := range p.read(t, p.r.alice) {
		if item.Kind == "proposal" {
			seen++
			wantBallot(t, item, "", false)
		}
	}
	if seen != 3 {
		t.Fatalf("outsider saw %d proposals, want 3", seen)
	}
}

func TestGetFeed_aResultItemHasNoBallotEvenWhenItsProposalDoes(t *testing.T) {
	t.Parallel()
	p := newBallotPage(t)
	result := feedFixture{pool: p.r.pool, gen: p.r.gen, clock: p.r.clock}.item(t, func(it *feed.Item) {
		it.Kind, it.RefID, it.CabalID, it.ActorID = feed.KindResult, p.r.proposal, ids.CabalIDFrom(p.r.cabal), p.r.bob
		it.Payload = feed.Payload{CabalName: "Alpha", Symbol: "AAPLx", ProposalID: p.r.proposal}
	})
	wantBallot(t, p.read(t, p.r.bob)[result], "", false)
}

func TestGetFeed_withoutProposalsTheBallotsPortIsNotCalled(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	feedFixture{pool: r.pool, gen: r.gen, clock: r.clock}.item(t, func(it *feed.Item) {
		it.Kind, it.CabalID, it.ActorID = feed.KindTrade, ids.CabalIDFrom(r.cabal), r.bob
		it.Payload = feed.Payload{CabalName: "Alpha", Symbol: "AAPLx", Action: feed.ActionBuy}
	})
	ballots := &fakeBallots{}
	f := feedFixture{pool: r.pool, gen: r.gen, clock: r.clock}
	routes := f.routesOptions(social.WithUsers(r.users), social.WithBallots(ballots))
	if _, err := routes.GetFeed(asUser(t.Context(), r.bob), api.GetFeedRequestObject{}); err != nil {
		t.Fatal(err)
	}
	if got := ballots.calls.Load(); got != 0 {
		t.Fatalf("MyBallots called %d times for a page without proposals, want 0", got)
	}
}

func TestGetFeed_withoutABallotsPortProposalsKeepTheDefaults(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	id := r.proposalItem(t, r.proposal, r.cabal)
	f := feedFixture{pool: r.pool, gen: r.gen, clock: r.clock}
	routes := f.routesOptions(social.WithUsers(r.users))
	res, err := routes.GetFeed(asUser(t.Context(), r.bob), api.GetFeedRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range res.(api.GetFeed200JSONResponse).Items {
		if item.Id == id {
			wantBallot(t, item, "", false)
			return
		}
	}
	t.Fatal("no proposal item on the page")
}

func TestGetFeed_failsWhenTheBallotsReadFails(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	r.proposalItem(t, r.proposal, r.cabal)
	f := feedFixture{pool: r.pool, gen: r.gen, clock: r.clock}
	routes := f.routesOptions(
		social.WithUsers(r.users), social.WithBallots(&fakeBallots{err: errs.New(errs.CodeDBUnavailable, "test")}))
	_, err := routes.GetFeed(asUser(t.Context(), r.bob), api.GetFeedRequestObject{})
	wantCode(t, err, errs.CodeDBUnavailable)
}

func TestGetFeedItem_aProposalCarriesTheViewersBallot(t *testing.T) {
	t.Parallel()
	p := newBallotPage(t)
	res, err := p.routes.GetFeedItem(asUser(t.Context(), p.r.bob), api.GetFeedItemRequestObject{Id: p.second})
	if err != nil {
		t.Fatal(err)
	}
	wantBallot(t, res.(api.GetFeedItem200JSONResponse).Item, "no", true)
}

type ballotProvider struct {
	module.Module
	ballots app.ViewerBallots
}

func (p ballotProvider) ViewerBallots() governanceport.ViewerBallots { return p.ballots }

func TestWire_takesTheViewerBallotsOfTheGovernanceModuleInTheSet(t *testing.T) {
	t.Parallel()
	p := newBallotPage(t)
	f := feedFixture{pool: p.r.pool, gen: p.r.gen, clock: p.r.clock}
	deps := module.Deps{Pool: f.pool, UoW: db.New(f.pool, f.gen, f.clock), IDs: f.gen, Clock: f.clock}
	wired := social.New(deps, social.WithUsers(p.r.users))
	wired.Wire(module.Set{ballotProvider{ballots: p.ballots}})
	res, err := social.HTTPOf(wired).GetFeed(asUser(t.Context(), p.r.bob), api.GetFeedRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range res.(api.GetFeed200JSONResponse).Items {
		if item.Id == p.second {
			wantBallot(t, item, "no", true)
		}
	}
	if p.ballots.calls.Load() != 1 {
		t.Fatalf("MyBallots called %d times, want 1", p.ballots.calls.Load())
	}
}
