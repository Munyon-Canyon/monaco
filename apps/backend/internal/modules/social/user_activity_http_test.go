package social_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking"
	rankingport "github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type activityCabals struct{ banned ids.CabalID }

func (c activityCabals) Cabals(_ context.Context, cabals []ids.CabalID) (map[ids.CabalID]cabalport.CabalView, error) {
	out := map[ids.CabalID]cabalport.CabalView{}
	for _, id := range cabals {
		status := cabalport.StatusActive
		if id == c.banned {
			status = cabalport.StatusBanned
		}
		out[id] = cabalport.CabalView{ID: id, Name: "AC Capital", Status: status}
	}
	return out, nil
}

type activityFixture struct {
	feedFixture
	viewer, member ids.UserID
	good, banned   ids.CabalID
	base           time.Time
}

func newActivityFixture(t *testing.T) activityFixture {
	t.Helper()
	f := newFeedFixture(t)
	return activityFixture{
		feedFixture: f, viewer: ids.NewUserID(f.gen), member: ids.NewUserID(f.gen),
		good: ids.CabalIDFrom(f.gen.NewV7()), banned: ids.CabalIDFrom(f.gen.NewV7()),
		base: time.Date(2026, time.October, 20, 12, 0, 0, 0, time.UTC),
	}
}

func (f activityFixture) deps() module.Deps {
	return module.Deps{Pool: f.pool, UoW: db.New(f.pool, f.gen, f.clock), IDs: f.gen, Clock: f.clock}
}

func (f activityFixture) socialModule() *social.Module {
	cards := []identityport.UserCard{
		{ID: f.viewer, AccountStatus: "active"}, {ID: f.member, AccountStatus: "active"},
	}
	return social.New(f.deps(), social.WithUsers(fakes.NewIdentity(cards, nil)),
		social.WithCabalPictures(activityCabals{banned: f.banned}))
}

func (f activityFixture) wiredRoutes() adapters.HTTP {
	d := f.deps()
	mod := f.socialModule()
	module.NewSet(mod, governance.New(d), ranking.New(d))
	return social.HTTPOf(mod)
}

func (f activityFixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func (f activityFixture) proposal(
	t *testing.T,
	cabal ids.CabalID,
	proposer ids.UserID,
	at time.Time,
	status string,
) uuid.UUID {
	t.Helper()
	id := f.gen.NewV7()
	f.exec(t, `INSERT INTO proposals (
			id, cabal_id, proposer_id, kind, symbol, mint, usdc_micros, quote_out_amount, threshold, status,
			expires_at, created_at, updated_at
		) VALUES ($1, $2, $3, 'buy', 'NVDA', 'mint', 1500000000, 1, 'majority', $4, $5, $5, $5)`,
		id, cabal.UUID(), proposer.UUID(), status, at)
	return id
}

func (f activityFixture) vote(t *testing.T, proposal uuid.UUID, voter ids.UserID, choice string, at time.Time) {
	t.Helper()
	f.exec(t, `INSERT INTO votes (proposal_id, voter_id, choice, cast_at, first_cast_at) VALUES ($1, $2, $3, $4, $4)`,
		proposal, voter.UUID(), choice, at)
}

func (f activityFixture) comment(
	t *testing.T,
	proposal uuid.UUID,
	cabal ids.CabalID,
	at time.Time,
	body, removal string,
) {
	t.Helper()
	item := f.item(t, func(it *feed.Item) { it.Kind, it.RefID, it.CabalID = feed.KindProposal, proposal, cabal })
	id := f.gen.NewV7()
	var deletedAt *time.Time
	var deletedBy *uuid.UUID
	if removal != "" {
		by := f.member.UUID()
		deletedAt, deletedBy = &at, &by
	} else {
		removal = "author"
	}
	f.exec(
		t,
		`INSERT INTO feed_comments (id, feed_object_id, author_id, body, created_at, deleted_at, deleted_by, deleted_by_kind)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		id,
		item,
		f.member.UUID(),
		body,
		at,
		deletedAt,
		deletedBy,
		removal,
	)
}

func (f activityFixture) topped(t *testing.T, month time.Time, right, resolved int) {
	t.Helper()
	f.exec(t, `INSERT INTO call_board_closed (
			board, period, period_start, rank, subject_id, rate_bps, right_count, resolved, voted, eligible, closed_at
		) VALUES ('people', 'month', $1, 1, $2, 9000, $3, $4, $4, $4, $5)`,
		month, f.member.UUID(), right, resolved, month.AddDate(0, 1, 0))
}

func (f activityFixture) activity(
	t *testing.T, routes adapters.HTTP, viewer ids.UserID, params api.GetUserActivityParams,
) api.UserActivityPage {
	t.Helper()
	res, err := routes.GetUserActivity(asUser(t.Context(), viewer), api.GetUserActivityRequestObject{
		Id: f.member.UUID(), Params: params,
	})
	if err != nil {
		t.Fatalf("GetUserActivity = %v", err)
	}
	page, ok := res.(api.GetUserActivity200JSONResponse)
	if !ok {
		t.Fatalf("response = %#v", res)
	}
	return api.UserActivityPage(page)
}

func (f activityFixture) walk(t *testing.T, routes adapters.HTTP, viewer ids.UserID, limit int) []api.UserActivity {
	t.Helper()
	var items []api.UserActivity
	var cursor *string
	for range 20 {
		page := f.activity(t, routes, viewer, api.GetUserActivityParams{Limit: &limit, Cursor: cursor})
		items = append(items, page.Items...)
		if cursor = page.NextCursor; cursor == nil {
			return items
		}
	}
	t.Fatal("the cursor never ended")
	return nil
}

func activityKinds(t *testing.T, items []api.UserActivity) []string {
	t.Helper()
	out := make([]string, len(items))
	for i, item := range items {
		kind, err := item.Discriminator()
		if err != nil {
			t.Fatal(err)
		}
		out[i] = kind
	}
	return out
}

func (f activityFixture) seedEverything(t *testing.T) (voted, proposed uuid.UUID) {
	t.Helper()
	other := ids.NewUserID(f.gen)
	elsewhere := f.proposal(t, f.good, other, f.base.Add(-48*time.Hour), "open")
	voted = elsewhere
	f.vote(t, elsewhere, f.member, "yes", f.base.Add(time.Hour))
	bannedProposal := f.proposal(t, f.banned, other, f.base.Add(-47*time.Hour), "open")
	f.vote(t, bannedProposal, f.member, "no", f.base.Add(90*time.Minute))
	proposed = f.proposal(t, f.good, f.member, f.base.Add(3*time.Hour), "passed")
	f.vote(t, proposed, other, "yes", f.base.Add(4*time.Hour))
	f.vote(t, proposed, f.viewer, "yes", f.base.Add(4*time.Hour))
	f.vote(t, proposed, ids.NewUserID(f.gen), "no", f.base.Add(4*time.Hour))
	f.comment(t, elsewhere, f.good, f.base.Add(2*time.Hour), "Concentration is a feature", "")
	f.comment(t, elsewhere, f.good, f.base.Add(150*time.Minute), "removed by a moderator", "admin")
	f.comment(t, elsewhere, f.good, f.base.Add(160*time.Minute), "deleted by its author", "author")
	f.topped(t, time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC), 11, 12)
	return voted, proposed
}

func TestGetUserActivity_servesTheFourKindsNewestFirstAcrossPagesThroughTheRealModuleSet(t *testing.T) {
	t.Parallel()
	f := newActivityFixture(t)
	f.seedEverything(t)
	routes := f.wiredRoutes()
	for _, limit := range []int{1, 2, 10} {
		items := f.walk(t, routes, f.viewer, limit)
		want := []string{"proposed", "replied", "voted", "topped_board"}
		if got := activityKinds(t, items); !slices.Equal(got, want) {
			t.Fatalf(
				"limit %d kinds = %v, want %v with the removed comments and the banned cabal's vote left out",
				limit,
				got,
				want,
			)
		}
	}
}

func TestGetUserActivity_describesProposalsAndReplies(t *testing.T) {
	t.Parallel()
	f := newActivityFixture(t)
	_, proposed := f.seedEverything(t)
	items := f.walk(t, f.wiredRoutes(), f.viewer, 10)
	p, _ := items[0].AsUserActivityProposed()
	if p.ProposalId != proposed || p.Subject != "Buy NVDA" || p.Status != "passed" || p.Result == nil ||
		p.Result.Yes != 2 || p.Result.No != 1 || p.Cabal.Name != "AC Capital" || p.Cabal.Id != f.good.UUID() {
		t.Errorf("proposed = %+v, want Buy NVDA passed 2-1 in AC Capital", p)
	}
}

func TestGetUserActivity_describesReplies(t *testing.T) {
	t.Parallel()
	f := newActivityFixture(t)
	voted, _ := f.seedEverything(t)
	items := f.walk(t, f.wiredRoutes(), f.viewer, 10)
	r, _ := items[1].AsUserActivityReplied()
	if r.Body != "Concentration is a feature" || r.Subject != "Buy NVDA" || r.UsdcMicros == nil ||
		*r.UsdcMicros != 1_500_000_000 || r.ProposalId != voted {
		t.Errorf("replied = %+v, want the comment on Buy NVDA $1,500", r)
	}
}

func TestGetUserActivity_describesVotesAndMonthWins(t *testing.T) {
	t.Parallel()
	f := newActivityFixture(t)
	voted, _ := f.seedEverything(t)
	items := f.walk(t, f.wiredRoutes(), f.viewer, 10)
	v, _ := items[2].AsUserActivityVoted()
	if v.ProposalId != voted || v.Title != "Voted" || v.Subject != "Buy NVDA" || v.Choice != nil {
		t.Errorf("voted = %+v, want a Voted row for Buy NVDA with no choice for another viewer", v)
	}
	top, _ := items[3].AsUserActivityToppedBoard()
	if top.Right != 11 || top.Resolved != 12 || top.PeriodStart.Format(time.DateOnly) != "2026-09-01" ||
		!top.At.Equal(time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("topped board = %+v, want September 11 of 12 dated October 1", top)
	}
}

func TestGetUserActivity_theMemberSeesTheirOwnChoiceAndNobodyElseDoes(t *testing.T) {
	t.Parallel()
	f := newActivityFixture(t)
	f.seedEverything(t)
	routes := f.wiredRoutes()
	for name, tt := range map[string]struct {
		viewer ids.UserID
		want   *api.UserActivityVotedChoice
	}{
		"another viewer": {f.viewer, nil},
		"the member":     {f.member, ptr(api.Yes)},
	} {
		items := f.walk(t, routes, tt.viewer, 10)
		voted, err := items[2].AsUserActivityVoted()
		if err != nil {
			t.Fatal(err)
		}
		if (voted.Choice == nil) != (tt.want == nil) || (voted.Choice != nil && *voted.Choice != *tt.want) {
			t.Errorf("%s sees choice %v, want %v", name, voted.Choice, tt.want)
		}
	}
}

func TestGetUserActivity_aBlockedPairGetsAnEmptyPageEitherWay(t *testing.T) {
	t.Parallel()
	f := newActivityFixture(t)
	f.seedEverything(t)
	routes := f.wiredRoutes()
	for name, pair := range map[string][2]ids.UserID{"viewer blocks member": {f.viewer, f.member}, "member blocks viewer": {f.member, f.viewer}} {
		f.exec(t, `INSERT INTO user_blocks (id, blocker_id, blocked_id, created_at) VALUES ($1, $2, $3, $4)`,
			f.gen.NewV7(), pair[0].UUID(), pair[1].UUID(), f.base)
		if page := f.activity(
			t,
			routes,
			f.viewer,
			api.GetUserActivityParams{},
		); len(page.Items) != 0 ||
			page.NextCursor != nil {
			t.Errorf("%s: page = %+v, want empty", name, page)
		}
		f.exec(t, `DELETE FROM user_blocks`)
	}
}

func TestGetUserActivity_withoutTheGovernanceAndRankingPicksItAnswersUpstreamUnavailable(t *testing.T) {
	t.Parallel()
	f := newActivityFixture(t)
	routes := social.HTTPOf(f.socialModule())
	_, err := routes.GetUserActivity(
		asUser(t.Context(), f.viewer),
		api.GetUserActivityRequestObject{Id: f.member.UUID()},
	)
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("err = %v, want upstream_unavailable", err)
	}
}

func TestGetUserActivity_refusesACursorTheServerDidNotWriteAndAnUnknownUser(t *testing.T) {
	t.Parallel()
	f := newActivityFixture(t)
	routes := f.wiredRoutes()
	bad := "not-a-cursor"
	_, err := routes.GetUserActivity(asUser(t.Context(), f.viewer), api.GetUserActivityRequestObject{
		Id: f.member.UUID(), Params: api.GetUserActivityParams{Cursor: &bad},
	})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Errorf("bad cursor err = %v, want invalid_input", err)
	}
	_, err = routes.GetUserActivity(asUser(t.Context(), f.viewer), api.GetUserActivityRequestObject{Id: f.gen.NewV7()})
	if errs.CodeOf(err) != errs.CodeUserNotFound {
		t.Errorf("unknown user err = %v, want user_not_found", err)
	}
}

func TestActivityStore_refusesALimitOutsideTheRangeAndReportsADeadDatabase(t *testing.T) {
	t.Parallel()
	f := newActivityFixture(t)
	store := app.NewActivityStore(f.pool)
	for _, limit := range []int{0, app.UserActivityPageMax + 1} {
		if _, err := store.Replies(t.Context(), f.member, nil, limit); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("limit %d err = %v, want invalid_input", limit, err)
		}
	}
	dead, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.Replies(
		dead,
		f.member,
		&governanceport.Cursor{At: f.base},
		5,
	); errs.CodeOf(
		err,
	) != errs.CodeInternal {
		t.Errorf("Replies on a dead context err = %v, want internal", err)
	}
	if _, err := store.BlockedEitherWay(dead, f.viewer, f.member); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("BlockedEitherWay on a dead context err = %v, want internal", err)
	}
}

type farFutureVote struct{ app.UnwiredMemberActivity }

func (farFutureVote) VotesBy(
	context.Context, ids.UserID, *governanceport.Cursor, int,
) ([]governanceport.CastVote, error) {
	return []governanceport.CastVote{
		{
			ProposalID: ids.ProposalIDFrom(
				uuid.UUID{1},
			),
			CabalID: ids.CabalIDFrom(uuid.UUID{2}),
			Kind:    "buy",
			Symbol:  "NVDA",
			Choice:  "yes",
			CastAt:  time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC),
		},
	}, nil
}

func (farFutureVote) ProposalsBy(
	context.Context, ids.UserID, *governanceport.Cursor, int,
) ([]governanceport.ProposedRow, error) {
	return []governanceport.ProposedRow{}, nil
}

func (farFutureVote) ProposalSubjects(
	context.Context, []ids.ProposalID,
) (map[ids.ProposalID]governanceport.Subject, error) {
	return map[ids.ProposalID]governanceport.Subject{}, nil
}

type noTops struct{}

func (noTops) ClosedMonthTops(context.Context, ids.UserID) ([]rankingport.ClosedTop, error) {
	return nil, nil
}

func TestGetUserActivity_aRowThatCannotBeEncodedFailsTheRead(t *testing.T) {
	t.Parallel()
	f := newActivityFixture(t)
	cards := []identityport.UserCard{
		{ID: f.viewer, AccountStatus: "active"}, {ID: f.member, AccountStatus: "active"},
	}
	routes := adapters.HTTP{Activity: &app.UserActivityQuery{
		Store: app.NewActivityStore(f.pool), Users: fakes.NewIdentity(cards, nil), Cabals: activityCabals{},
		Activity: farFutureVote{}, Tops: noTops{},
	}}
	_, err := routes.GetUserActivity(
		asUser(t.Context(), f.viewer),
		api.GetUserActivityRequestObject{Id: f.member.UUID()},
	)
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("err = %v, want internal", err)
	}
}

func TestGetUserActivity_requiresASignedInUser(t *testing.T) {
	t.Parallel()
	f := newActivityFixture(t)
	_, err := f.wiredRoutes().GetUserActivity(t.Context(), api.GetUserActivityRequestObject{Id: f.member.UUID()})
	if errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Errorf("err = %v, want unauthorized", err)
	}
}
