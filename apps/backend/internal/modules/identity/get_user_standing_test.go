package identity_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	rankingport "github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/identityapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type pairBlocks map[[2]ids.UserID]bool

func (b pairBlocks) Counts(context.Context, ids.UserID) (int, int, error) { return 0, 0, nil }

func (b pairBlocks) FollowedByMe(context.Context, ids.UserID, ids.UserID) (bool, error) {
	return false, nil
}

func (b pairBlocks) FollowedAmong(context.Context, ids.UserID, []ids.UserID) (map[ids.UserID]bool, error) {
	return map[ids.UserID]bool{}, nil
}

func (b pairBlocks) BlockedByMe(_ context.Context, blocker, blocked ids.UserID) (bool, error) {
	return b[[2]ids.UserID{blocker, blocked}], nil
}

type userCabals map[ids.UserID][]ids.CabalID

func (c userCabals) CabalsOf(_ context.Context, user ids.UserID) ([]ids.CabalID, error) {
	return c[user], nil
}

type standingFixture struct {
	viewer, target ids.UserID
	cabals         userCabals
	blocks         pairBlocks
	standing       rankingport.Standing
	shareReturns   bool
	now            time.Time
}

func newStandingFixture() *standingFixture {
	gen := testkit.NewIDs(6467)
	now := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	return &standingFixture{
		viewer: ids.NewUserID(gen), target: ids.NewUserID(gen), cabals: userCabals{}, blocks: pairBlocks{},
		shareReturns: true, now: now,
		standing: rankingport.Standing{
			Rank: intp(8), Of: 24, RateBps: intp(7800), Right: 7, Resolved: 9,
			PeriodStart: time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC),
		},
	}
}

func intp(v int) *int { return &v }

func ptr[T any](v T) *T { return &v }

func (f *standingFixture) profile(t *testing.T, viewer ids.UserID) api.GetUser200JSONResponse {
	t.Helper()
	cards := fakes.NewIdentity([]app.UserCard{
		{ID: f.target, Handle: "target", SharePerformance: f.shareReturns},
		{ID: f.viewer, Handle: "viewer", SharePerformance: f.shareReturns},
	}, nil)
	pnl := rankingport.PersonReturn{PnlMicros: money.SignedMicrosFromInt64(241_100_000)}
	routes := adapters.HTTP{
		Cards: cards, Follows: f.blocks, Cabals: f.cabals,
		Returns:   stubReturns{byUser: map[ids.UserID]rankingport.PersonReturn{f.target: pnl, f.viewer: pnl}},
		Standings: stubStandings{row: f.standing}, Clock: testkit.NewClock(f.now),
	}
	return getUser(t, routes, viewer, f.target)
}

func TestGetUser_servesTheRankedMonthStanding(t *testing.T) {
	t.Parallel()
	f := newStandingFixture()
	got := f.profile(t, f.viewer)
	s := got.Standing
	if s == nil || s.Period != api.Month || s.PeriodStart.Format("2006-01-02") != "2026-10-01" || got.Crown {
		t.Fatalf("standing = %+v, crown = %v", s, got.Crown)
	}
	want := api.ProfileStanding{
		Right: 7, Resolved: 9, RankedCount: 24, MoreToRank: 0, Rank: ptr(int32(8)), RateBps: ptr(int32(7800)),
	}
	if *s.Rank != *want.Rank || *s.RateBps != *want.RateBps || s.Right != want.Right ||
		s.Resolved != want.Resolved || s.RankedCount != want.RankedCount || s.MoreToRank != want.MoreToRank {
		t.Fatalf("standing = %+v, want %+v", s, want)
	}
}

func TestGetUser_servesAnUnrankedStandingWithHowManyCallsRemain(t *testing.T) {
	t.Parallel()
	f := newStandingFixture()
	f.standing.Rank, f.standing.RateBps, f.standing.Right, f.standing.Resolved = nil, nil, 2, 3
	s := f.profile(t, f.viewer).Standing
	if s == nil || s.Rank != nil || s.RateBps != nil || s.MoreToRank != rankingport.MinResolvedCalls-3 ||
		s.Resolved != 3 {
		t.Fatalf("unranked standing = %+v, want null rank and rate, more_to_rank 2", s)
	}
}

func TestGetUser_leavesTheStandingNullWithNoCallsThisMonth(t *testing.T) {
	t.Parallel()
	f := newStandingFixture()
	f.standing = rankingport.Standing{PeriodStart: f.standing.PeriodStart}
	if got := f.profile(t, f.viewer); got.Standing != nil || got.Crown {
		t.Fatalf("no-calls standing = %+v, crown = %v, want null", got.Standing, got.Crown)
	}
}

func TestGetUser_crownsOnlyRankOne(t *testing.T) {
	t.Parallel()
	f := newStandingFixture()
	f.standing.Rank = intp(1)
	if got := f.profile(t, f.viewer); !got.Crown {
		t.Fatal("rank 1 has no crown")
	}
	f.standing.Rank = intp(2)
	if got := f.profile(t, f.viewer); got.Crown {
		t.Fatal("rank 2 has a crown")
	}
}

func TestGetUser_countsCabalsInCommonFromLiveMemberships(t *testing.T) {
	t.Parallel()
	gen := testkit.NewIDs(64670)
	cabal := func() ids.CabalID { return ids.CabalIDFrom(gen.NewV7()) }
	shared1, shared2, mineOnly, theirsOnly := cabal(), cabal(), cabal(), cabal()
	f := newStandingFixture()
	f.cabals[f.viewer] = []ids.CabalID{shared1, shared2, mineOnly}
	f.cabals[f.target] = []ids.CabalID{shared2, shared1, theirsOnly}
	if got := f.profile(t, f.viewer); got.CabalsInCommon != 2 || got.CabalCount != 3 {
		t.Fatalf("cabals in common = %d of %d, want 2 of 3", got.CabalsInCommon, got.CabalCount)
	}
	f.cabals[f.viewer] = []ids.CabalID{shared2, mineOnly}
	if got := f.profile(t, f.viewer); got.CabalsInCommon != 1 {
		t.Fatalf("after leaving one, cabals in common = %d, want 1", got.CabalsInCommon)
	}
	f.cabals[f.target] = f.cabals[f.viewer]
	if got := f.profile(t, f.target); got.CabalsInCommon != 0 {
		t.Fatalf("own profile cabals in common = %d, want 0", got.CabalsInCommon)
	}
}

func TestGetUser_nullsTheStandingWhenEitherSideBlocked(t *testing.T) {
	t.Parallel()
	for name, pair := range map[string]func(f *standingFixture) [2]ids.UserID{
		"viewer blocked member": func(f *standingFixture) [2]ids.UserID { return [2]ids.UserID{f.viewer, f.target} },
		"member blocked viewer": func(f *standingFixture) [2]ids.UserID { return [2]ids.UserID{f.target, f.viewer} },
	} {
		f := newStandingFixture()
		f.standing.Rank = intp(1)
		f.blocks[pair(f)] = true
		if got := f.profile(t, f.viewer); got.Standing != nil || got.Crown {
			t.Fatalf("%s: standing = %+v, crown = %v, want null", name, got.Standing, got.Crown)
		}
	}
}

func TestGetUser_nullsAStandingFromBeforeTheCurrentMonth(t *testing.T) {
	t.Parallel()
	f := newStandingFixture()
	f.now = time.Date(2026, time.November, 1, 0, 5, 0, 0, time.UTC)
	if got := f.profile(t, f.viewer); got.Standing != nil || got.Crown {
		t.Fatalf("October row on Nov 1 = %+v, want null", got.Standing)
	}
	f.standing.PeriodStart = time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC)
	got := f.profile(t, f.viewer)
	if got.Standing == nil || got.Standing.PeriodStart.Format("2006-01-02") != "2026-11-01" {
		t.Fatalf("November row on Nov 1 = %+v, want period_start 2026-11-01", got.Standing)
	}
}

func TestGetUser_hidesPerformanceFromOthersButNotFromTheOwnerOrTheStanding(t *testing.T) {
	t.Parallel()
	f := newStandingFixture()
	f.shareReturns = false
	other := f.profile(t, f.viewer)
	if other.Pnl1mMicros != nil || !other.ReturnPrivate || other.Standing == nil {
		t.Fatalf("other viewer = pnl %v, private %v, standing %+v, want null pnl and a standing",
			other.Pnl1mMicros, other.ReturnPrivate, other.Standing)
	}
	own := f.profile(t, f.target)
	if own.Pnl1mMicros == nil || *own.Pnl1mMicros != 241_100_000 || !own.ReturnPrivate || own.Standing == nil {
		t.Fatalf("own profile = pnl %v, private %v, standing %+v, want the P&L, private true", own.Pnl1mMicros,
			own.ReturnPrivate, own.Standing)
	}
}

func TestGetUser_failuresInTheStandingsPort(t *testing.T) {
	t.Parallel()
	f := newStandingFixture()
	cards := fakes.NewIdentity([]app.UserCard{{ID: f.target, Handle: "target"}}, nil)
	for name, standings := range map[string]app.Standings{
		"unwired": app.UnwiredStandings{}, "down": stubStandings{err: errs.New(errs.CodeInternal, "test")},
	} {
		routes := adapters.HTTP{
			Cards: cards, Follows: f.blocks, Cabals: f.cabals, Returns: stubReturns{}, Standings: standings,
			Clock: testkit.NewClock(f.now),
		}
		if _, err := routes.GetUser(asUser(t, f.viewer), api.GetUserRequestObject{Id: f.target.UUID()}); err == nil {
			t.Fatalf("standings %s: GetUser succeeded", name)
		}
	}
}
