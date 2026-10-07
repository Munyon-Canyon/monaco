package social_test

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func TestPostUserBlock_blocksAndAnswersNoContent(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for range 2 {
		res, err := f.routes().
			PostUserBlock(asUser(t.Context(), f.alice), api.PostUserBlockRequestObject{Id: f.bob.UUID()})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := res.(api.PostUserBlock204Response); !ok {
			t.Fatalf("response = %#v, want 204", res)
		}
	}
	if n := f.eventCount(t, events.TypeBlockCreated); n != 1 {
		t.Fatalf("block.created events = %d, want 1", n)
	}
}

func TestDeleteUserBlock_unblocksAndAnswersNoContent(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.block(t, f.alice, f.bob); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		res, err := f.routes().DeleteUserBlock(
			asUser(t.Context(), f.alice), api.DeleteUserBlockRequestObject{Id: f.bob.UUID()})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := res.(api.DeleteUserBlock204Response); !ok {
			t.Fatalf("response = %#v, want 204", res)
		}
	}
	if n := f.eventCount(t, events.TypeBlockRemoved); n != 1 {
		t.Fatalf("block.removed events = %d, want 1", n)
	}
}

func TestPostUserBlock_passesTheCommandRefusalThrough(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	_, err := f.routes().PostUserBlock(asUser(t.Context(), f.alice), api.PostUserBlockRequestObject{Id: f.alice.UUID()})
	wantCode(t, err, errs.CodeCannotBlockSelf)
}

func TestBlockRoutes_refuseACallerThatIsNotASignedInUser(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	tests := []struct {
		name  string
		actor *auth.Actor
		want  errs.Code
	}{
		{"no actor", nil, errs.CodeUnauthorized},
		{"admin actor", &auth.Actor{Kind: auth.ActorAdmin, ID: f.alice.String()}, errs.CodeForbidden},
		{"malformed actor id", &auth.Actor{Kind: auth.ActorUser, ID: "not-a-uuid"}, errs.CodeUnauthorized},
	}
	for _, tt := range tests {
		ctx := t.Context()
		if tt.actor != nil {
			ctx = auth.WithActor(ctx, *tt.actor)
		}
		_, err := f.routes().PostUserBlock(ctx, api.PostUserBlockRequestObject{Id: f.bob.UUID()})
		wantCode(t, err, tt.want)
		_, err = f.routes().DeleteUserBlock(ctx, api.DeleteUserBlockRequestObject{Id: f.bob.UUID()})
		wantCode(t, err, tt.want)
		_, err = f.routes().GetMeBlocks(ctx, api.GetMeBlocksRequestObject{})
		wantCode(t, err, tt.want)
	}
}

func TestBlockRoutes_refuseAPathIdThatIsNotAUserId(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := asUser(t.Context(), f.alice)
	var v4 openapi_types.UUID
	copy(v4[:], []byte("0123456789abcdef"))
	_, err := f.routes().PostUserBlock(ctx, api.PostUserBlockRequestObject{Id: v4})
	wantCode(t, err, errs.CodeInvalidInput)
	_, err = f.routes().DeleteUserBlock(ctx, api.DeleteUserBlockRequestObject{Id: v4})
	wantCode(t, err, errs.CodeInvalidInput)
}

func TestDeleteUserBlock_passesTheUnblockFailureThrough(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE user_blocks CASCADE`); err != nil {
		t.Fatal(err)
	}
	_, err := f.routes().
		DeleteUserBlock(asUser(t.Context(), f.alice), api.DeleteUserBlockRequestObject{Id: f.bob.UUID()})
	wantCode(t, err, errs.CodeInternal)
}

func TestGetMeBlocks_listsNewestFirstAndKeepsBannedAndDeletedUsers(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for _, target := range []ids.UserID{f.bob, f.banned} {
		f.clock.Advance(time.Second)
		if err := f.block(t, f.alice, target); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.pool.Exec(t.Context(),
		`INSERT INTO user_blocks (id, blocker_id, blocked_id, created_at) VALUES ($1, $2, $3, $4)`,
		f.gen.NewV7(), f.alice.UUID(), f.deleted.UUID(), f.now.Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := f.block(t, f.bob, f.alice); err != nil {
		t.Fatal(err)
	}
	res, err := f.routes().GetMeBlocks(asUser(t.Context(), f.alice), api.GetMeBlocksRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := res.(api.GetMeBlocks200JSONResponse)
	if !ok {
		t.Fatalf("response = %#v, want 200", res)
	}
	type listed struct {
		id     uuid.UUID
		handle string
	}
	have := make([]listed, 0, len(got.Users))
	for _, u := range got.Users {
		have = append(have, listed{u.UserId, u.Handle})
	}
	want := []listed{{f.deleted.UUID(), ""}, {f.banned.UUID(), "mallory"}, {f.bob.UUID(), "bob"}}
	if !slices.Equal(have, want) {
		t.Fatalf("users = %+v, want the deleted, banned and suspended users newest first: %+v", have, want)
	}
}

func TestGetMeBlocks_answersAnEmptyListWithoutLookingUpUsers(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.users.Fail("UsersByID", errs.New(errs.CodeInternal, "test"))
	res, err := f.routes().GetMeBlocks(asUser(t.Context(), f.alice), api.GetMeBlocksRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := res.(api.GetMeBlocks200JSONResponse); !ok || got.Users == nil || len(got.Users) != 0 {
		t.Fatalf("response = %#v, want an empty users list", res)
	}
}

func TestGetMeBlocks_failsWhenTheStoreOrTheUserLookupFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.block(t, f.alice, f.bob); err != nil {
		t.Fatal(err)
	}
	boom := errs.New(errs.CodeUpstreamUnavailable, "test")
	f.users.Fail("UsersByID", boom)
	_, err := f.routes().GetMeBlocks(asUser(t.Context(), f.alice), api.GetMeBlocksRequestObject{})
	wantCode(t, err, errs.CodeUpstreamUnavailable)
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE user_blocks CASCADE`); err != nil {
		t.Fatal(err)
	}
	_, err = f.routes().GetMeBlocks(asUser(t.Context(), f.alice), api.GetMeBlocksRequestObject{})
	wantCode(t, err, errs.CodeInternal)
}
