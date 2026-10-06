package social_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type commentRoutes struct {
	commentFixture
	routes adapters.HTTP
}

func newCommentRoutes(t *testing.T) commentRoutes {
	t.Helper()
	f := commentRoutes{commentFixture: newCommentFixture(t)}
	users := fakes.NewIdentity([]identity.UserCard{
		{ID: f.cabal.Members[0].ID, Handle: "kai", DisplayName: "Kai", PhotoURL: "https://cdn.example.com/kai.jpg"},
		{ID: f.cabal.Members[1].ID, Handle: "gone", DisplayName: "Gone", Deleted: true},
		{ID: f.outsider, Handle: "mia", DisplayName: "Mia"},
	}, nil)
	deps := module.Deps{Pool: f.pool, UoW: f.uow, IDs: f.gen, Clock: f.clock}
	f.routes = social.HTTPOf(social.New(deps, social.WithUsers(users)))
	return f
}

func (f commentRoutes) post(
	t *testing.T, item uuid.UUID, author ids.UserID, body *api.CreateCommentRequest,
) (api.Comment, error) {
	t.Helper()
	res, err := f.routes.PostFeedComment(asUser(t.Context(), author),
		api.PostFeedCommentRequestObject{Id: item, Body: body})
	if err != nil {
		return api.Comment{}, err
	}
	created, ok := res.(api.PostFeedComment201JSONResponse)
	if !ok {
		t.Fatalf("response = %#v", res)
	}
	return api.Comment(created), nil
}

func TestPostFeedComment_returnsTheCommentWithItsAuthor(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	kai := f.cabal.Members[0].ID
	top, err := f.post(t, f.item(t, feed.KindTrade), kai, &api.CreateCommentRequest{Body: " first "})
	if err != nil {
		t.Fatal(err)
	}
	body := "first"
	want := api.Comment{
		Id: top.Id, Author: api.CommentAuthor{
			Id: kai.UUID(), Handle: ptr("kai"), DisplayName: "Kai", PhotoUrl: ptr("https://cdn.example.com/kai.jpg"),
		},
		Body: &body, BodyDisplay: "first", IsMine: true, CreatedAt: f.clock.Now(),
	}
	if !reflect.DeepEqual(top, want) {
		t.Fatalf("comment = %+v, want %+v", top, want)
	}
}

func TestPostFeedComment_aReplyToAReplyNamesWhoItAnswers(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	item := f.item(t, feed.KindTrade)
	kai, gone := f.cabal.Members[0].ID, f.cabal.Members[1].ID
	top, err := f.post(t, item, kai, &api.CreateCommentRequest{Body: "top"})
	if err != nil {
		t.Fatal(err)
	}
	byKai, err := f.post(t, item, kai, &api.CreateCommentRequest{Body: "ok", ParentCommentId: &top.Id})
	if err != nil {
		t.Fatal(err)
	}
	byGone, err := f.post(t, item, gone, &api.CreateCommentRequest{Body: "hm", ParentCommentId: &top.Id})
	if err != nil {
		t.Fatal(err)
	}
	toKai, err := f.post(t, item, f.outsider, &api.CreateCommentRequest{Body: "yes", ParentCommentId: &byKai.Id})
	if err != nil {
		t.Fatal(err)
	}
	toGone, err := f.post(t, item, f.outsider, &api.CreateCommentRequest{Body: "no", ParentCommentId: &byGone.Id})
	if err != nil {
		t.Fatal(err)
	}
	if toKai.ParentCommentId == nil || *toKai.ParentCommentId != top.Id ||
		toKai.ReplyToHandle == nil || *toKai.ReplyToHandle != "kai" {
		t.Errorf("reply to a reply = %+v, want parent %s and reply_to_handle kai", toKai, top.Id)
	}
	if toGone.ReplyToHandle != nil {
		t.Errorf("reply to a deleted account = %+v, want no reply_to_handle", toGone)
	}
}

func TestPostFeedComment_refusesBadInput(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	item := f.item(t, feed.KindTrade)
	for name, body := range map[string]*api.CreateCommentRequest{
		"no body": nil, "blank": {Body: "  "},
	} {
		if _, err := f.post(t, item, f.outsider, body); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("%s: error = %v, want invalid_input", name, err)
		}
	}
	if _, err := f.routes.PostFeedComment(t.Context(), api.PostFeedCommentRequestObject{
		Id: item, Body: &api.CreateCommentRequest{Body: "hi"},
	}); errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Errorf("anonymous: error = %v, want unauthorized", err)
	}
}

func TestPostFeedComment_passesTheCommandAndIdentityErrorsThrough(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	_, err := f.post(t, f.gen.NewV7(), f.outsider, &api.CreateCommentRequest{Body: "hi"})
	if errs.CodeOf(err) != errs.CodeFeedItemNotFound {
		t.Errorf("unknown item: error = %v, want feed_item_not_found", err)
	}
	deps := module.Deps{Pool: f.pool, UoW: f.uow, IDs: f.gen, Clock: f.clock}
	down := social.HTTPOf(social.New(deps, social.WithUsers(brokenUsers{})))
	_, err = down.PostFeedComment(asUser(t.Context(), f.outsider), api.PostFeedCommentRequestObject{
		Id: f.item(t, feed.KindTrade), Body: &api.CreateCommentRequest{Body: "hi"},
	})
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Errorf("identity down: error = %v, want upstream_unavailable", err)
	}
}

type brokenUsers struct{}

func (brokenUsers) UsersByID(context.Context, []ids.UserID) (map[ids.UserID]port.UserCard, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "test")
}

func TestPostFeedComment_aDeletedAuthorShowsOnlyTheId(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	gone := f.cabal.Members[1].ID
	got, err := f.post(t, f.item(t, feed.KindTrade), gone, &api.CreateCommentRequest{Body: "hm"})
	if err != nil {
		t.Fatal(err)
	}
	if want := (api.CommentAuthor{Id: gone.UUID()}); got.Author != want {
		t.Fatalf("author = %+v, want %+v", got.Author, want)
	}
}
