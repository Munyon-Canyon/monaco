package social_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
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
