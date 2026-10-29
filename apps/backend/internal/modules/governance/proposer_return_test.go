package governance_test

import (
	"context"
	"sync"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	rankingport "github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type cardBook struct {
	mu    sync.Mutex
	cards map[ids.UserID]identityport.UserCard
	calls [][]ids.UserID
	err   error
}

func (c *cardBook) UsersByID(_ context.Context, who []ids.UserID) (map[ids.UserID]identityport.UserCard, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, who)
	return c.cards, c.err
}

type returnBook struct {
	rankingport.Queries

	mu      sync.Mutex
	returns map[ids.UserID]rankingport.PersonReturn
	ranges  []rankingport.Range
	err     error
}

func (r *returnBook) PeopleReturns(
	_ context.Context, rng rankingport.Range, _ []ids.UserID,
) (map[ids.UserID]rankingport.PersonReturn, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ranges = append(r.ranges, rng)
	return r.returns, r.err
}

func TestGetProposal_ProposerReturn(t *testing.T) {
	t.Parallel()
	bps := int64(820)
	for name, tc := range map[string]struct {
		cards   func(ids.UserID) map[ids.UserID]identityport.UserCard
		returns func(ids.UserID) map[ids.UserID]rankingport.PersonReturn
		wantBps *int64
		private bool
	}{
		"a proposer who shares shows the figure": {
			cards: func(id ids.UserID) map[ids.UserID]identityport.UserCard {
				return map[ids.UserID]identityport.UserCard{id: {ID: id, SharePerformance: true}}
			},
			returns: func(id ids.UserID) map[ids.UserID]rankingport.PersonReturn {
				return map[ids.UserID]rankingport.PersonReturn{id: {ReturnBps: &bps}}
			},
			wantBps: &bps,
		},
		"a proposer who hides it is private with no figure": {
			cards: func(id ids.UserID) map[ids.UserID]identityport.UserCard {
				return map[ids.UserID]identityport.UserCard{id: {ID: id}}
			},
			returns: func(id ids.UserID) map[ids.UserID]rankingport.PersonReturn {
				return map[ids.UserID]rankingport.PersonReturn{id: {ReturnBps: &bps}}
			},
			private: true,
		},
		"a proposer with no ranked row is neither": {
			cards: func(id ids.UserID) map[ids.UserID]identityport.UserCard {
				return map[ids.UserID]identityport.UserCard{id: {ID: id, SharePerformance: true}}
			},
			returns: func(ids.UserID) map[ids.UserID]rankingport.PersonReturn { return nil },
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newReadsDB(t)
			p := d.buy(d.caller)
			p.CabalID = d.cabal
			d.insert(t, p)
			proposer := ids.UserIDFrom(p.ProposerID)
			users := &cardBook{cards: tc.cards(proposer)}
			rets := &returnBook{returns: tc.returns(proposer)}
			reads := app.NewProposalReads(d.pool, d.swaps, d.names, d.assets, users, rets, &replyBook{})
			got, err := reads.Get(t.Context(), app.GetProposal{
				ID: ids.ProposalIDFrom(p.ID), Caller: ids.UserIDFrom(d.caller),
			})
			if err != nil {
				t.Fatal(err)
			}
			checkReturn(t, got, tc.wantBps, tc.private)
			if len(users.calls) != 1 || users.calls[0][0] != proposer {
				t.Fatalf("UsersByID calls = %v, want one call that starts with the proposer", users.calls)
			}
			if len(rets.ranges) != 1 || rets.ranges[0] != rankingport.Range1M {
				t.Fatalf("PeopleReturns ranges = %v, want one 1M call", rets.ranges)
			}
		})
	}
}

func checkReturn(t *testing.T, got app.ProposalDetail, want *int64, private bool) {
	t.Helper()
	same := (got.ProposerReturnBps == nil) == (want == nil) &&
		(want == nil || *got.ProposerReturnBps == *want)
	if !same || got.ProposerReturnPrivate != private {
		t.Fatalf("return = %v private %v, want %v private %v",
			got.ProposerReturnBps, got.ProposerReturnPrivate, want, private)
	}
}

func TestGetProposal_ProposerReturnPortFailure(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeDBUnavailable, "t")
	for name, tc := range map[string]struct {
		users *cardBook
		rets  *returnBook
	}{
		"the card read":   {users: &cardBook{err: boom}, rets: &returnBook{}},
		"the return read": {users: &cardBook{}, rets: &returnBook{err: boom}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newReadsDB(t)
			p := d.buy(d.caller)
			p.CabalID = d.cabal
			d.insert(t, p)
			reads := app.NewProposalReads(d.pool, d.swaps, d.names, d.assets, tc.users, tc.rets, &replyBook{})
			_, err := reads.Get(t.Context(), app.GetProposal{
				ID: ids.ProposalIDFrom(p.ID), Caller: ids.UserIDFrom(d.caller),
			})
			if errs.CodeOf(err) != errs.CodeDBUnavailable {
				t.Fatalf("Get = %v, want %v", err, errs.CodeDBUnavailable)
			}
		})
	}
}
