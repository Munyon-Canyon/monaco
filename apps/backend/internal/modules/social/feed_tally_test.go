package social_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type fakeTallies struct {
	calls atomic.Int32
	votes map[ids.ProposalID]app.Tally
	err   error
}

func (f *fakeTallies) Tallies(_ context.Context, proposals []ids.ProposalID) (map[ids.ProposalID]app.Tally, error) {
	f.calls.Add(1)
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[ids.ProposalID]app.Tally, len(proposals))
	for _, id := range proposals {
		if t, ok := f.votes[id]; ok {
			out[id] = t
		}
	}
	return out, nil
}

func (r proposalRig) proposalItem(t *testing.T, id uuid.UUID, cabal uuid.UUID) uuid.UUID {
	t.Helper()
	f := feedFixture{pool: r.pool, gen: r.gen, clock: r.clock}
	return f.item(t, func(it *feed.Item) {
		it.Kind, it.RefID, it.CabalID, it.ActorID = feed.KindProposal, id, ids.CabalIDFrom(cabal), r.bob
		it.Payload = feed.Payload{
			CabalName: "Alpha", Symbol: "AAPLx", AssetName: "Apple", Action: feed.ActionBuy, ProposalID: id,
			ExpiresAt: r.now.Add(time.Hour),
		}
	})
}

type tallyPage struct {
	r       proposalRig
	routes  adapters.HTTP
	tallies *fakeTallies
	first   uuid.UUID
	second  uuid.UUID
	closes  time.Time
}

func newTallyPage(t *testing.T) tallyPage {
	t.Helper()
	r := newProposalRig(t)
	other := r.gen.NewV7()
	p := tallyPage{r: r, closes: r.now.Add(4 * time.Hour)}
	p.first, p.second = r.proposalItem(t, r.proposal, r.cabal), r.proposalItem(t, other, r.cabal)
	feedFixture{pool: r.pool, gen: r.gen, clock: r.clock}.item(t, func(it *feed.Item) {
		it.Kind, it.CabalID, it.ActorID = feed.KindTrade, ids.CabalIDFrom(r.cabal), r.bob
		it.Payload = feed.Payload{CabalName: "Alpha", Symbol: "AAPLx", Action: feed.ActionBuy}
	})
	p.tallies = &fakeTallies{votes: map[ids.ProposalID]app.Tally{
		ids.ProposalIDFrom(r.proposal): {Yes: 4, No: 1, Voters: 8, Needed: 5, ExpiresAt: p.closes},
		ids.ProposalIDFrom(other):      {Yes: 0, No: 0, Voters: 3, Needed: 2, ExpiresAt: p.closes},
	}}
	f := feedFixture{pool: r.pool, gen: r.gen, clock: r.clock}
	p.routes = f.routesOptions(social.WithUsers(r.users), social.WithTallies(p.tallies))
	return p
}

func (p tallyPage) read(t *testing.T) map[uuid.UUID]api.FeedItem {
	t.Helper()
	res, err := p.routes.GetFeed(asUser(t.Context(), p.r.bob), api.GetFeedRequestObject{Params: api.GetFeedParams{}})
	if err != nil {
		t.Fatal(err)
	}
	out := map[uuid.UUID]api.FeedItem{}
	for _, item := range res.(api.GetFeed200JSONResponse).Items {
		out[item.Id] = item
	}
	return out
}

func TestGetFeed_aProposalCarriesItsLiveTallyFromOneCallPerPage(t *testing.T) {
	t.Parallel()
	p := newTallyPage(t)
	items := p.read(t)
	if got := p.tallies.calls.Load(); got != 1 {
		t.Fatalf("Tallies called %d times for one page, want 1", got)
	}
	got := items[p.first]
	if got.Tally == nil || *got.Tally != (api.FeedTally{Yes: 4, No: 1, Voters: 8, Needed: 5}) ||
		got.ExpiresAt == nil || !got.ExpiresAt.Equal(p.closes) {
		t.Fatalf("first proposal tally %+v expires %v, want 4 1 8 5 closing %v", got.Tally, got.ExpiresAt, p.closes)
	}
	if items[p.second].Tally == nil || items[p.second].Tally.Voters != 3 {
		t.Fatalf("second proposal tally = %+v, want 3 voters", items[p.second].Tally)
	}
	for _, item := range items {
		if item.Kind == "trade" && (item.Tally != nil || item.ExpiresAt != nil) {
			t.Fatalf("trade carries tally %+v expires %v, want both null", item.Tally, item.ExpiresAt)
		}
	}
}

func TestGetFeed_aBallotChangesTheNextReadWithoutWritingTheFeed(t *testing.T) {
	t.Parallel()
	p := newTallyPage(t)
	updatedAt := func() time.Time {
		var at time.Time
		err := p.r.pool.QueryRow(t.Context(), `SELECT max(updated_at) FROM feed_objects`).Scan(&at)
		if err != nil {
			t.Fatal(err)
		}
		return at
	}
	before := updatedAt()
	p.read(t)
	ballot := app.Tally{Yes: 5, No: 1, Voters: 8, Needed: 5, ExpiresAt: p.closes}
	p.tallies.votes[ids.ProposalIDFrom(p.r.proposal)] = ballot
	if yes := p.read(t)[p.first].Tally.Yes; yes != 5 {
		t.Fatalf("tally yes = %d after a ballot, want 5", yes)
	}
	if after := updatedAt(); !after.Equal(before) {
		t.Fatalf("feed_objects.updated_at moved from %v to %v, want no write", before, after)
	}
}

func TestGetFeed_aProposalWithoutATallyKeepsThePayloadClosingTime(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	r.proposalItem(t, r.proposal, r.cabal)
	f := feedFixture{pool: r.pool, gen: r.gen, clock: r.clock}
	routes := f.routesOptions(social.WithUsers(r.users), social.WithTallies(&fakeTallies{}))
	res, err := routes.GetFeed(asUser(t.Context(), r.bob), api.GetFeedRequestObject{Params: api.GetFeedParams{}})
	if err != nil {
		t.Fatal(err)
	}
	want := r.now.Add(time.Hour)
	for _, item := range res.(api.GetFeed200JSONResponse).Items {
		if item.Kind != "proposal" {
			continue
		}
		if item.Tally != nil || item.ExpiresAt == nil || !item.ExpiresAt.Equal(want) {
			t.Fatalf("proposal tally %+v expires %v, want no tally closing %v", item.Tally, item.ExpiresAt, want)
		}
		return
	}
	t.Fatal("no proposal item on the page")
}

func TestGetFeed_failsWhenTheTallyReadFails(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	r.proposalItem(t, r.proposal, r.cabal)
	f := feedFixture{pool: r.pool, gen: r.gen, clock: r.clock}
	routes := f.routesOptions(
		social.WithUsers(r.users), social.WithTallies(&fakeTallies{err: errs.New(errs.CodeDBUnavailable, "test")}))
	_, err := routes.GetFeed(asUser(t.Context(), r.bob), api.GetFeedRequestObject{Params: api.GetFeedParams{}})
	wantCode(t, err, errs.CodeDBUnavailable)
}

func TestFeedVote_everyBallotPublishesOneGlobalHintAfterCommit(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	sub := testkit.SubscribeCore(t, r.conn, hintSubject)
	cast := events.VoteCast{
		V: 1, ProposalID: r.proposal, CabalID: r.cabal, VoterID: r.bob.UUID(), Choice: "yes", CastAt: r.now,
	}
	retracted := events.VoteRetracted{
		V: 1, ProposalID: r.proposal, CabalID: r.cabal, VoterID: r.bob.UUID(), Choice: "yes", RetractedAt: r.now,
	}
	for _, fn := range []func(context.Context, db.Tx) error{
		func(ctx context.Context, tx db.Tx) error { return r.feed.VoteCast(ctx, tx, cast, r.now) },
		func(ctx context.Context, tx db.Tx) error { return r.feed.VoteRetracted(ctx, tx, retracted, r.now) },
	} {
		if err := r.deliver(t, fn); err != nil {
			t.Fatal(err)
		}
	}
	wantHints(t, r.conn.Conn, sub, 2)
	rolled := r.deliver(t, func(ctx context.Context, tx db.Tx) error {
		if err := r.feed.VoteCast(ctx, tx, cast, r.now); err != nil {
			return err
		}
		return errs.New(errs.CodeInternal, "rollback")
	})
	if rolled == nil {
		t.Fatal("rollback returned nil")
	}
	wantHints(t, r.conn.Conn, sub, 0)
}

type tallyProvider struct {
	module.Module
	tallies app.Tallies
}

func (p tallyProvider) Tallies() governanceport.Tallies { return p.tallies }

func TestWire_takesTheTalliesOfTheGovernanceModuleInTheSet(t *testing.T) {
	t.Parallel()
	p := newTallyPage(t)
	f := feedFixture{pool: p.r.pool, gen: p.r.gen, clock: p.r.clock}
	deps := module.Deps{Pool: f.pool, UoW: db.New(f.pool, f.gen, f.clock), IDs: f.gen, Clock: f.clock}
	wired := social.New(deps, social.WithUsers(p.r.users))
	wired.Wire(module.Set{tallyProvider{tallies: p.tallies}})
	res, err := social.HTTPOf(wired).GetFeed(asUser(t.Context(), p.r.bob), api.GetFeedRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range res.(api.GetFeed200JSONResponse).Items {
		if item.Id == p.first && (item.Tally == nil || item.Tally.Yes != 4) {
			t.Fatalf("tally = %+v, want the wired provider's 4 yes", item.Tally)
		}
	}
	if p.tallies.calls.Load() != 1 {
		t.Fatalf("Tallies called %d times, want 1", p.tallies.calls.Load())
	}
}
