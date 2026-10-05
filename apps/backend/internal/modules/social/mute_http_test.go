package social_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
)

func TestFeedMuteRoutes_writeAndDelete(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	routes := f.routes()
	ctx := asUser(t.Context(), f.alice)
	request := api.PutMeFeedMutesRequestObject{
		Body: &api.FeedMuteRequest{TargetType: "kind", TargetId: "trade"},
	}
	res, err := routes.PutMeFeedMutes(ctx, request)
	if err != nil || res != (api.PutMeFeedMutes204Response{}) {
		t.Fatalf("put = %#v, %v", res, err)
	}
	deleteRequest := api.DeleteMeFeedMutesTargetTypeTargetIDRequestObject{TargetType: "kind", TargetId: "trade"}
	deleted, err := routes.DeleteMeFeedMutesTargetTypeTargetID(ctx, deleteRequest)
	if err != nil || deleted != (api.DeleteMeFeedMutesTargetTypeTargetID204Response{}) {
		t.Fatalf("delete = %#v, %v", deleted, err)
	}
	_, err = routes.PutMeFeedMutes(ctx, api.PutMeFeedMutesRequestObject{})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("err = %v", err)
	}
	_, err = routes.PutMeFeedMutes(t.Context(), request)
	if errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("err = %v", err)
	}
	badRequest := api.PutMeFeedMutesRequestObject{
		Body: &api.FeedMuteRequest{TargetType: "kind", TargetId: "bad"},
	}
	_, err = routes.PutMeFeedMutes(ctx, badRequest)
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("err = %v", err)
	}
	_, err = routes.DeleteMeFeedMutesTargetTypeTargetID(t.Context(), deleteRequest)
	if errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("err = %v", err)
	}
	invalid := api.DeleteMeFeedMutesTargetTypeTargetIDRequestObject{TargetType: "kind", TargetId: "bad"}
	_, err = routes.DeleteMeFeedMutesTargetTypeTargetID(ctx, invalid)
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("err = %v", err)
	}
}

func TestGetMeFeedMutes_listsAndHandlesFailures(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := asUser(t.Context(), f.alice)
	routes := f.routes()
	if err := f.mute.Handle(f.ctx(t), app.Mute{User: f.alice, TargetType: "kind", TargetID: "trade"}); err != nil {
		t.Fatal(err)
	}
	res, err := routes.GetMeFeedMutes(ctx, api.GetMeFeedMutesRequestObject{})
	if err != nil || len(res.(api.GetMeFeedMutes200JSONResponse)) != 1 {
		t.Fatalf("%v %v", res, err)
	}
	_, err = routes.GetMeFeedMutes(t.Context(), api.GetMeFeedMutesRequestObject{})
	if errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE feed_mutes`); err != nil {
		t.Fatal(err)
	}
	_, err = routes.GetMeFeedMutes(ctx, api.GetMeFeedMutesRequestObject{})
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatal(err)
	}
}
