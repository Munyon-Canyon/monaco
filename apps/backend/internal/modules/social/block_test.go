package social_test

import (
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (f fixture) block(t *testing.T, blocker, blocked ids.UserID) error {
	t.Helper()
	handler := app.NewBlockUserHandler(app.BlockUserDeps{
		UoW: db.New(f.pool, f.gen, f.clock), Users: f.users, IDs: f.gen, Clock: f.clock,
	})
	return handler.Handle(f.ctx(t), app.BlockUser{Blocker: blocker, Blocked: blocked})
}

func (f fixture) unblock(t *testing.T, blocker, blocked ids.UserID) error {
	t.Helper()
	handler := app.NewUnblockUserHandler(db.New(f.pool, f.gen, f.clock), f.clock)
	return handler.Handle(f.ctx(t), app.UnblockUser{Blocker: blocker, Blocked: blocked})
}

func (f fixture) followBetween(t *testing.T, follower, followee ids.UserID) {
	t.Helper()
	cmd := app.Follow{Follower: follower, Followee: followee, Source: domain.SourceProfile}
	if err := f.follow.Handle(f.ctx(t), cmd); err != nil {
		t.Fatal(err)
	}
}

func (f fixture) blockRows(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM user_blocks`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestBlockUser_endsBothFollowsAndAppendsEventsInOneTransaction(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.followBetween(t, f.alice, f.bob)
	f.followBetween(t, f.bob, f.alice)
	if err := f.block(t, f.alice, f.bob); err != nil {
		t.Fatal(err)
	}
	var (
		block     uuid.UUID
		created   time.Time
		payload   []byte
		aggregate uuid.UUID
		actor     string
	)
	err := f.pool.QueryRow(t.Context(),
		`SELECT b.id, b.created_at, e.payload, e.aggregate_id, e.actor_type || ':' || e.actor_id
		 FROM user_blocks b, events e WHERE e.type = 'block.created'`).
		Scan(&block, &created, &payload, &aggregate, &actor)
	if err != nil {
		t.Fatal(err)
	}
	var got events.BlockCreated
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	want := events.BlockCreated{
		V: 1, BlockID: block, BlockerID: f.alice.UUID(), BlockedID: f.bob.UUID(), CreatedAt: f.now,
	}
	if got != want || !created.Equal(f.now) || aggregate != block || actor != "user:"+f.alice.String() {
		t.Fatalf("event = %+v, row at %s, aggregate %s, actor %s; want %+v at %s",
			got, created, aggregate, actor, want, f.now)
	}
	if live, _ := f.rows(t); live != 0 {
		t.Fatalf("live follows = %d, want both ended", live)
	}
	if n := f.eventCount(t, events.TypeFollowRemoved); n != 2 {
		t.Fatalf("follow.removed events = %d, want 2", n)
	}
}

func TestBlockUser_withNoFollowAppendsOnlyBlockCreated(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.block(t, f.alice, f.bob); err != nil {
		t.Fatal(err)
	}
	if n := f.eventCount(t, events.TypeFollowRemoved); n != 0 {
		t.Fatalf("follow.removed events = %d, want 0", n)
	}
	if n := f.eventCount(t, events.TypeBlockCreated); n != 1 {
		t.Fatalf("block.created events = %d, want 1", n)
	}
}

func TestBlockUser_aRepeatChangesNothing(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for range 2 {
		if err := f.block(t, f.alice, f.bob); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.blockRows(t); n != 1 {
		t.Fatalf("blocks = %d, want 1", n)
	}
	if n := f.eventCount(t, events.TypeBlockCreated); n != 1 {
		t.Fatalf("block.created events = %d, want 1", n)
	}
}

func TestBlockUser_aRepeatEndsNoFollow(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.block(t, f.alice, f.bob); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(),
		`INSERT INTO follows (id, follower_id, followee_id, source, created_at) VALUES ($1, $2, $3, 'profile', now())`,
		f.gen.NewV7(), f.alice.UUID(), f.bob.UUID()); err != nil {
		t.Fatal(err)
	}
	if err := f.block(t, f.alice, f.bob); err != nil {
		t.Fatal(err)
	}
	if live, _ := f.rows(t); live != 1 {
		t.Fatalf("live follows = %d, want the repeat to change nothing", live)
	}
}

func TestBlockUser_blocksEachDirectionOnItsOwn(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.block(t, f.alice, f.bob); err != nil {
		t.Fatal(err)
	}
	if err := f.block(t, f.bob, f.alice); err != nil {
		t.Fatal(err)
	}
	if n := f.blockRows(t); n != 2 {
		t.Fatalf("blocks = %d, want one per direction", n)
	}
}

func TestBlockUser_refusesBeforeWritingAnything(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	tests := []struct {
		name    string
		blocked ids.UserID
		want    errs.Code
	}{
		{"yourself", f.alice, errs.CodeCannotBlockSelf},
		{"an unknown user", f.stranger, errs.CodeUserNotFound},
		{"a deleted user", f.deleted, errs.CodeUserNotFound},
	}
	for _, tt := range tests {
		wantCode(t, f.block(t, f.alice, tt.blocked), tt.want)
	}
	if n := f.blockRows(t); n != 0 {
		t.Fatalf("blocks = %d, want 0", n)
	}
	if n := f.eventCount(t, events.TypeBlockCreated); n != 0 {
		t.Fatalf("block.created events = %d, want 0", n)
	}
}

func TestBlockUser_aBannedOrSuspendedUserCanBeBlocked(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for _, target := range []ids.UserID{f.banned, f.bob} {
		if err := f.block(t, f.alice, target); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.blockRows(t); n != 2 {
		t.Fatalf("blocks = %d, want 2", n)
	}
}

func TestBlockUser_returnsTheUserLookupFailure(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	boom := errs.New(errs.CodeInternal, "test")
	f.users.Fail("UsersByID", boom)
	if err := f.block(t, f.alice, f.bob); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the lookup error", err)
	}
}

func TestBlockUser_failsWithInternalAndRollsBackWhenAWriteFails(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		sql  string
	}{
		{"the block cannot be stored", `DROP TABLE user_blocks CASCADE`},
		{"a follow cannot be ended", `DROP TABLE follows CASCADE`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			if _, err := f.pool.Exec(t.Context(), tt.sql); err != nil {
				t.Fatal(err)
			}
			wantCode(t, f.block(t, f.alice, f.bob), errs.CodeInternal)
			if n := f.eventCount(t, events.TypeBlockCreated); n != 0 {
				t.Fatalf("block.created events = %d, want 0", n)
			}
		})
	}
}

func TestBlockUser_failsAndRollsBackWhenAnEventCannotBeAppended(t *testing.T) {
	t.Parallel()
	for _, withFollow := range []bool{false, true} {
		t.Run(strconv.FormatBool(withFollow), func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			if withFollow {
				f.followBetween(t, f.alice, f.bob)
			}
			if _, err := f.pool.Exec(t.Context(), `DROP TABLE events CASCADE`); err != nil {
				t.Fatal(err)
			}
			if err := f.block(t, f.alice, f.bob); err == nil {
				t.Fatal("Block = nil, want the append failure")
			}
			if n := f.blockRows(t); n != 0 {
				t.Fatalf("blocks = %d, want the insert rolled back", n)
			}
		})
	}
}

func TestUnblockUser_deletesTheRowAndAppendsBlockRemoved(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.block(t, f.alice, f.bob); err != nil {
		t.Fatal(err)
	}
	var block uuid.UUID
	if err := f.pool.QueryRow(t.Context(), `SELECT id FROM user_blocks`).Scan(&block); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := f.unblock(t, f.alice, f.bob); err != nil {
			t.Fatal(err)
		}
	}
	var payload []byte
	if err := f.pool.QueryRow(t.Context(),
		`SELECT payload FROM events WHERE type = 'block.removed'`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var got events.BlockRemoved
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	want := events.BlockRemoved{
		V: 1, BlockID: block, BlockerID: f.alice.UUID(), BlockedID: f.bob.UUID(), RemovedAt: f.now,
	}
	if got != want || f.blockRows(t) != 0 {
		t.Fatalf("event = %+v, blocks = %d; want %+v and none", got, f.blockRows(t), want)
	}
}

func TestUnblockUser_restoresNoFollowAndLeavesTheOtherDirection(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.followBetween(t, f.alice, f.bob)
	if err := f.block(t, f.alice, f.bob); err != nil {
		t.Fatal(err)
	}
	if err := f.block(t, f.bob, f.alice); err != nil {
		t.Fatal(err)
	}
	if err := f.unblock(t, f.alice, f.bob); err != nil {
		t.Fatal(err)
	}
	if live, _ := f.rows(t); live != 0 {
		t.Fatalf("live follows = %d, want none restored", live)
	}
	if n := f.blockRows(t); n != 1 {
		t.Fatalf("blocks = %d, want bob's block kept", n)
	}
}

func TestUnblockUser_failsAndRollsBackWhenTheStoreFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.block(t, f.alice, f.bob); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE events CASCADE`); err != nil {
		t.Fatal(err)
	}
	if err := f.unblock(t, f.alice, f.bob); err == nil {
		t.Fatal("Unblock = nil, want the append failure")
	}
	if n := f.blockRows(t); n != 1 {
		t.Fatalf("blocks = %d, want the delete rolled back", n)
	}
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE user_blocks CASCADE`); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.unblock(t, f.alice, f.bob), errs.CodeInternal)
}
