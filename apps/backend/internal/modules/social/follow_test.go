package social_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type fixture struct {
	now      time.Time
	gen      *testkit.IDs
	pool     *pgxpool.Pool
	clock    *testkit.Clock
	users    *fakes.Identity
	follow   *app.FollowHandler
	unfollow *app.UnfollowHandler
	mute     *app.MuteHandler
	unmute   *app.UnmuteHandler
	alice    ids.UserID
	bob      ids.UserID
	banned   ids.UserID
	deleted  ids.UserID
	stranger ids.UserID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	g := testkit.NewIDs(569)
	pool := testkit.DB(t)
	now := clock.Real{}.Now().UTC().Truncate(time.Second)
	clk := testkit.NewClock(now)
	uow := db.New(pool, g, clk)
	user := func() ids.UserID { return ids.NewUserID(g) }
	f := fixture{
		now: now, gen: g, pool: pool, clock: clk,
		alice: user(), bob: user(), banned: user(), deleted: user(), stranger: user(),
	}
	f.users = fakes.NewIdentity([]identity.UserCard{
		{ID: f.alice, Handle: "alice", AccountStatus: identity.AccountActive},
		{ID: f.bob, Handle: "bob", AccountStatus: identity.AccountSuspended},
		{ID: f.banned, Handle: "mallory", AccountStatus: identity.AccountBanned},
		{ID: f.deleted, AccountStatus: identity.AccountDeleted, Deleted: true},
	}, nil)
	f.follow = app.NewFollowHandler(app.FollowDeps{UoW: uow, Users: f.users, IDs: g, Clock: clk})
	f.unfollow = app.NewUnfollowHandler(uow, clk)
	f.mute = app.NewMuteHandler(uow, clk)
	f.unmute = app.NewUnmuteHandler(uow)
	return f
}

func (f fixture) ctx(t *testing.T) context.Context {
	t.Helper()
	return observability.WithActor(t.Context(), "user:"+f.alice.String())
}

func (f fixture) follows(t *testing.T, source domain.FollowSource) error {
	t.Helper()
	return f.follow.Handle(f.ctx(t), app.Follow{Follower: f.alice, Followee: f.bob, Source: source})
}

func (f fixture) unfollows(t *testing.T) error {
	t.Helper()
	return f.unfollow.Handle(f.ctx(t), app.Unfollow{Follower: f.alice, Followee: f.bob})
}

func (f fixture) rows(t *testing.T) (live, total int) {
	t.Helper()
	err := f.pool.QueryRow(t.Context(),
		`SELECT count(*) FILTER (WHERE deleted_at IS NULL), count(*) FROM follows`).Scan(&live, &total)
	if err != nil {
		t.Fatal(err)
	}
	return live, total
}

func (f fixture) eventCount(t *testing.T, typ events.Type) int {
	t.Helper()
	var n int
	err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE type = $1`, string(typ)).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func wantCode(t *testing.T, err error, code errs.Code) {
	t.Helper()
	if got := errs.CodeOf(err); err == nil || got != code {
		t.Fatalf("err = %v (code %s), want %s", err, got, code)
	}
}

func TestFollow_insertsTheRowAndAppendsFollowCreatedInOneTransaction(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.follows(t, domain.SourceFeed); err != nil {
		t.Fatal(err)
	}
	var (
		follow   uuid.UUID
		source   string
		created  time.Time
		actor    string
		payload  []byte
		aggregte uuid.UUID
	)
	err := f.pool.QueryRow(t.Context(),
		`SELECT f.id, f.source, f.created_at, e.actor_type || ':' || e.actor_id, e.payload, e.aggregate_id
		 FROM follows f, events e WHERE e.type = 'follow.created'`).
		Scan(&follow, &source, &created, &actor, &payload, &aggregte)
	if err != nil {
		t.Fatal(err)
	}
	var got events.FollowCreated
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	want := events.FollowCreated{
		V: 1, FollowID: follow, FollowerID: f.alice.UUID(), FolloweeID: f.bob.UUID(), Source: "feed", CreatedAt: f.now,
	}
	wantActor := "user:" + f.alice.String()
	if got != want || source != "feed" || !created.Equal(f.now) || aggregte != follow || actor != wantActor {
		t.Fatalf("event = %+v, row source %q at %s, aggregate %s, actor %s; want %+v at %s",
			got, source, created, aggregte, actor, want, f.now)
	}
}

func TestFollow_twiceLeavesOneLiveRowAndOneEvent(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for range 2 {
		if err := f.follows(t, domain.SourceProfile); err != nil {
			t.Fatal(err)
		}
	}
	live, total := f.rows(t)
	if n := f.eventCount(t, events.TypeFollowCreated); live != 1 || total != 1 || n != 1 {
		t.Fatalf("live %d, total %d, follow.created %d; want 1, 1, 1", live, total, n)
	}
}

func TestFollowUnfollowFollow_keepsEachCycleAsItsOwnRow(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for _, unfollow := range []bool{false, true, false} {
		f.clock.Advance(time.Minute)
		err := f.follows(t, domain.SourceProfile)
		if unfollow {
			err = f.unfollows(t)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	live, total := f.rows(t)
	created, removed := f.eventCount(t, events.TypeFollowCreated), f.eventCount(t, events.TypeFollowRemoved)
	if live != 1 || total != 2 || created != 2 || removed != 1 {
		t.Fatalf("live %d, total %d, created %d, removed %d; want 1, 2, 2, 1", live, total, created, removed)
	}
	var deletedAt time.Time
	if err := f.pool.QueryRow(t.Context(), `SELECT deleted_at FROM follows WHERE deleted_at IS NOT NULL`).
		Scan(&deletedAt); err != nil {
		t.Fatal(err)
	}
	if want := f.now.Add(2 * time.Minute); !deletedAt.Equal(want) {
		t.Fatalf("deleted_at = %s, want the unfollow's clock reading %s", deletedAt, want)
	}
}

func TestUnfollow_withNoLiveFollowSucceedsAndAppendsNothing(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.unfollows(t); err != nil {
		t.Fatal(err)
	}
	if n := f.eventCount(t, events.TypeFollowRemoved); n != 0 {
		t.Fatalf("follow.removed events = %d, want 0", n)
	}
}

func TestUnfollow_aBannedOrDeletedUserCanStillBeUnfollowed(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for _, target := range []ids.UserID{f.banned, f.deleted} {
		if _, err := f.pool.Exec(t.Context(),
			`INSERT INTO follows (id, follower_id, followee_id, created_at) VALUES ($1, $2, $3, $4)`,
			f.gen.NewV7(), f.alice.UUID(), target.UUID(), f.now); err != nil {
			t.Fatal(err)
		}
		if err := f.unfollow.Handle(f.ctx(t), app.Unfollow{Follower: f.alice, Followee: target}); err != nil {
			t.Fatal(err)
		}
	}
	if live, _ := f.rows(t); live != 0 {
		t.Fatalf("live follows = %d, want 0", live)
	}
}

func TestFollow_refusesBeforeWritingAnything(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	tests := []struct {
		name     string
		followee ids.UserID
		want     errs.Code
	}{
		{"self", f.alice, errs.CodeCannotFollowSelf},
		{"unknown user", f.stranger, errs.CodeUserNotFound},
		{"deleted user", f.deleted, errs.CodeUserNotFound},
		{"banned user", f.banned, errs.CodeUserBanned},
	}
	for _, tt := range tests {
		cmd := app.Follow{Follower: f.alice, Followee: tt.followee, Source: domain.SourceProfile}
		err := f.follow.Handle(f.ctx(t), cmd)
		if got := errs.CodeOf(err); err == nil || got != tt.want {
			t.Errorf("%s: err = %v (code %s), want %s", tt.name, err, got, tt.want)
		}
	}
	if _, total := f.rows(t); total != 0 {
		t.Fatalf("follows = %d, want 0", total)
	}
}

func TestFollow_returnsTheUserLookupFailure(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	boom := errs.New(errs.CodeInternal, "test")
	f.users.Fail("UsersByID", boom)
	if err := f.follows(t, domain.SourceProfile); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the lookup error", err)
	}
}

func TestCommands_failWithInternalWhenTheStoreFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE follows`); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.follows(t, domain.SourceProfile), errs.CodeInternal)
	wantCode(t, f.unfollows(t), errs.CodeInternal)
}

func TestFollow_failsAndRollsBackWhenTheEventCannotBeAppended(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE events CASCADE`); err != nil {
		t.Fatal(err)
	}
	if err := f.follows(t, domain.SourceX); err == nil {
		t.Fatal("Follow = nil, want the append failure")
	}
	if _, total := f.rows(t); total != 0 {
		t.Fatalf("follows = %d, want the insert rolled back", total)
	}
}

func TestUnfollow_failsAndRollsBackWhenTheEventCannotBeAppended(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.follows(t, domain.SourceProfile); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE events CASCADE`); err != nil {
		t.Fatal(err)
	}
	if err := f.unfollows(t); err == nil {
		t.Fatal("Unfollow = nil, want the append failure")
	}
	if live, _ := f.rows(t); live != 1 {
		t.Fatalf("live follows = %d, want the update rolled back", live)
	}
}
