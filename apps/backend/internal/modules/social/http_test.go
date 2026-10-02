package social_test

import (
	"context"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func (f fixture) routes() httpx.Routes {
	deps := module.Deps{Pool: f.pool, UoW: db.New(f.pool, f.gen, f.clock), IDs: f.gen, Clock: f.clock}
	var routes httpx.Routes
	social.New(deps, social.WithUsers(f.users)).Routes(&routes)
	return routes
}

func asUser(ctx context.Context, id ids.UserID) context.Context {
	return auth.WithActor(ctx, auth.Actor{Kind: auth.ActorUser, ID: id.String(), Standing: auth.StandingActive})
}

func followReq(id ids.UserID, source *string) api.PostUserFollowRequestObject {
	req := api.PostUserFollowRequestObject{Id: id.UUID()}
	if source != nil {
		req.Body = &api.FollowRequest{Source: source}
	}
	return req
}

func TestPostUserFollow_followsAndReportsFollowing(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	feed := "feed"
	for _, body := range []*string{nil, &feed} {
		res, err := f.routes().PostUserFollow(asUser(t.Context(), f.alice), followReq(f.bob, body))
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := res.(api.PostUserFollow200JSONResponse); !ok || !got.Following {
			t.Fatalf("response = %#v, want following true", res)
		}
	}
	var source string
	err := f.pool.QueryRow(t.Context(), `SELECT source FROM follows WHERE deleted_at IS NULL`).Scan(&source)
	if err != nil {
		t.Fatal(err)
	}
	if source != "profile" {
		t.Fatalf("source = %q, want the first call's default profile", source)
	}
}

func TestPostUserFollow_refusesASourceTheClientMayNotSend(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	referral := "referral"
	_, err := f.routes().PostUserFollow(asUser(t.Context(), f.alice), followReq(f.bob, &referral))
	wantCode(t, err, errs.CodeInvalidInput)
	if _, total := f.rows(t); total != 0 {
		t.Fatalf("follows = %d, want 0", total)
	}
}

func TestPostUserFollow_passesTheCommandRefusalThrough(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	_, err := f.routes().PostUserFollow(asUser(t.Context(), f.alice), followReq(f.alice, nil))
	wantCode(t, err, errs.CodeCannotFollowSelf)
}

func TestDeleteUserFollow_unfollowsAndReportsNotFollowing(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.follows(t, "profile"); err != nil {
		t.Fatal(err)
	}
	req := api.DeleteUserFollowRequestObject{Id: f.bob.UUID()}
	for range 2 {
		res, err := f.routes().DeleteUserFollow(asUser(t.Context(), f.alice), req)
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := res.(api.DeleteUserFollow200JSONResponse); !ok || got.Following {
			t.Fatalf("response = %#v, want following false", res)
		}
	}
	if n := f.eventCount(t, events.TypeFollowRemoved); n != 1 {
		t.Fatalf("follow.removed events = %d, want 1", n)
	}
}

func TestRoutes_refuseACallerThatIsNotASignedInUser(t *testing.T) {
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
		_, err := f.routes().PostUserFollow(ctx, followReq(f.bob, nil))
		if got := errs.CodeOf(err); err == nil || got != tt.want {
			t.Errorf("follow, %s: err = %v (code %s), want %s", tt.name, err, got, tt.want)
		}
		_, err = f.routes().DeleteUserFollow(ctx, api.DeleteUserFollowRequestObject{})
		if got := errs.CodeOf(err); err == nil || got != tt.want {
			t.Errorf("unfollow, %s: err = %v (code %s), want %s", tt.name, err, got, tt.want)
		}
	}
}

func TestRoutes_refuseAPathIdThatIsNotAUserId(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := asUser(t.Context(), f.alice)
	var v4 openapi_types.UUID
	copy(v4[:], []byte("0123456789abcdef"))
	_, err := f.routes().PostUserFollow(ctx, api.PostUserFollowRequestObject{Id: v4})
	wantCode(t, err, errs.CodeInvalidInput)
	_, err = f.routes().DeleteUserFollow(ctx, api.DeleteUserFollowRequestObject{Id: v4})
	wantCode(t, err, errs.CodeInvalidInput)
}

func TestRoutes_passTheUnfollowFailureThrough(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE follows`); err != nil {
		t.Fatal(err)
	}
	req := api.DeleteUserFollowRequestObject{Id: f.bob.UUID()}
	_, err := f.routes().DeleteUserFollow(asUser(t.Context(), f.alice), req)
	wantCode(t, err, errs.CodeInternal)
}
