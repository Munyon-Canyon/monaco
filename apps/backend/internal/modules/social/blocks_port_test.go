package social_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestFollowsPort_BlockedByMe(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	port := f.port()
	want := func(viewer, user bool, label string) {
		t.Helper()
		got, err := port.BlockedByMe(t.Context(), f.alice, f.bob)
		if err != nil || got != viewer {
			t.Fatalf("%s: alice blocks bob = %v, %v; want %v", label, got, err, viewer)
		}
		got, err = port.BlockedByMe(t.Context(), f.bob, f.alice)
		if err != nil || got != user {
			t.Fatalf("%s: bob blocks alice = %v, %v; want %v", label, got, err, user)
		}
	}
	want(false, false, "before any block")
	if err := f.block(t, f.alice, f.bob); err != nil {
		t.Fatal(err)
	}
	want(true, false, "after alice blocks bob")
	if err := f.unblock(t, f.alice, f.bob); err != nil {
		t.Fatal(err)
	}
	want(false, false, "after the unblock")
}

func TestFollowsPort_BlockedByMeFailsWhenTheStoreFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE user_blocks CASCADE`); err != nil {
		t.Fatal(err)
	}
	_, err := f.port().BlockedByMe(t.Context(), f.alice, f.bob)
	wantCode(t, err, errs.CodeInternal)
}

func TestBlocksPort_BlockedEitherWayWhicheverSideBlocked(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	blocks := social.New(module.Deps{Pool: f.pool}, social.WithUsers(f.users)).Blocks()
	want := func(blocked bool, label string) {
		t.Helper()
		for _, pair := range [][2]ids.UserID{{f.alice, f.bob}, {f.bob, f.alice}} {
			got, err := blocks.BlockedEitherWay(t.Context(), pair[0], pair[1])
			if err != nil || got != blocked {
				t.Fatalf("%s: BlockedEitherWay = %v, %v; want %v", label, got, err, blocked)
			}
		}
	}
	want(false, "before any block")
	if err := f.block(t, f.alice, f.bob); err != nil {
		t.Fatal(err)
	}
	want(true, "after alice blocks bob")
}
