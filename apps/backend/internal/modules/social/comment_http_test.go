package social_test

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"time"

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
	"github.com/monaco/monaco/apps/backend/internal/testkit"
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

func (f commentRoutes) read(
	t *testing.T,
	viewer ids.UserID,
	item uuid.UUID,
	p api.GetFeedCommentsParams,
) api.CommentPage {
	t.Helper()
	res, err := f.routes.GetFeedComments(
		asUser(t.Context(), viewer),
		api.GetFeedCommentsRequestObject{Id: item, Params: p},
	)
	if err != nil {
		t.Fatal(err)
	}
	page, ok := res.(api.GetFeedComments200JSONResponse)
	if !ok {
		t.Fatalf("response = %#v", res)
	}
	return api.CommentPage(page)
}

func (f commentRoutes) say(
	t *testing.T, item uuid.UUID, author ids.UserID, body string, parent *uuid.UUID,
) api.Comment {
	t.Helper()
	c, err := f.post(t, item, author, &api.CreateCommentRequest{Body: body, ParentCommentId: parent})
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Second)
	return c
}

func TestGetFeedComments_rendersADeletedCommentInPlaceAndMarksOwnComments(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	item := f.item(t, feed.KindTrade)
	kai := f.cabal.Members[0].ID
	top := f.say(t, item, kai, "top", nil)
	reply := f.say(t, item, f.outsider, "reply", &top.Id)
	if _, err := f.routes.DeleteFeedComment(asUser(t.Context(), kai), api.DeleteFeedCommentRequestObject{
		CommentId: top.Id,
	}); err != nil {
		t.Fatal(err)
	}
	page := f.read(t, f.outsider, item, api.GetFeedCommentsParams{})
	if len(page.Items) != 1 || len(page.Items[0].Replies) != 1 {
		t.Fatalf("page = %+v, want one thread with a reply", page)
	}
	gotTop, gotReply := page.Items[0].Comment, page.Items[0].Replies[0]
	if gotTop.Body != nil || gotTop.BodyDisplay != "Comment deleted" || gotTop.IsMine ||
		gotTop.Author.Id != kai.UUID() {
		t.Errorf("deleted top-level comment = %+v", gotTop)
	}
	if gotReply.Id != reply.Id || gotReply.Body == nil || *gotReply.Body != "reply" || !gotReply.IsMine {
		t.Errorf("reply = %+v", gotReply)
	}
}

func TestGetFeedComments_pagesWithAnOpaqueCursor(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	item := f.item(t, feed.KindTrade)
	f.say(t, item, f.outsider, "one", nil)
	f.say(t, item, f.outsider, "two", nil)
	first := f.read(t, f.outsider, item, api.GetFeedCommentsParams{Limit: ptr(1)})
	if len(first.Items) != 1 || first.NextCursor == nil || first.Items[0].Comment.BodyDisplay != "one" {
		t.Fatalf("first page = %+v", first)
	}
	rest := f.read(t, f.outsider, item, api.GetFeedCommentsParams{Cursor: first.NextCursor})
	if len(rest.Items) != 1 || rest.Items[0].Comment.BodyDisplay != "two" || rest.NextCursor != nil {
		t.Fatalf("second page = %+v", rest)
	}
}

func TestGetFeedComments_refusesBadInput(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	ctx := asUser(t.Context(), f.outsider)
	_, err := f.routes.GetFeedComments(ctx, api.GetFeedCommentsRequestObject{
		Id: f.item(t, feed.KindTrade), Params: api.GetFeedCommentsParams{Cursor: ptr("nope")},
	})
	wantCode(t, err, errs.CodeInvalidInput)
	_, err = f.routes.GetFeedComments(ctx, api.GetFeedCommentsRequestObject{Id: f.gen.NewV7()})
	wantCode(t, err, errs.CodeFeedItemNotFound)
	_, err = f.routes.GetFeedComments(t.Context(), api.GetFeedCommentsRequestObject{Id: f.gen.NewV7()})
	wantCode(t, err, errs.CodeUnauthorized)
}

func TestGetFeedComments_passesIdentityErrorsThrough(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	item := f.item(t, feed.KindTrade)
	if _, err := f.post(t, item, f.outsider, &api.CreateCommentRequest{Body: "hi"}); err != nil {
		t.Fatal(err)
	}
	deps := module.Deps{Pool: f.pool, UoW: f.uow, IDs: f.gen, Clock: f.clock}
	down := social.HTTPOf(social.New(deps, social.WithUsers(brokenUsers{})))
	_, err := down.GetFeedComments(asUser(t.Context(), f.outsider), api.GetFeedCommentsRequestObject{Id: item})
	wantCode(t, err, errs.CodeUpstreamUnavailable)
}

func TestDeleteFeedComment_answers204AndPassesRefusalsThrough(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	kai := f.cabal.Members[0].ID
	top, err := f.post(t, f.item(t, feed.KindTrade), kai, &api.CreateCommentRequest{Body: "top"})
	if err != nil {
		t.Fatal(err)
	}
	req := api.DeleteFeedCommentRequestObject{CommentId: top.Id}
	_, err = f.routes.DeleteFeedComment(asUser(t.Context(), f.outsider), req)
	wantCode(t, err, errs.CodeCommentNotAuthor)
	for range 2 {
		res, err := f.routes.DeleteFeedComment(asUser(t.Context(), kai), req)
		if _, ok := res.(api.DeleteFeedComment204Response); err != nil || !ok {
			t.Fatalf("delete = %#v, %v, want 204", res, err)
		}
	}
	_, err = f.routes.DeleteFeedComment(t.Context(), req)
	wantCode(t, err, errs.CodeUnauthorized)
}

func TestGetFeedItem_reportsWhetherTheCallerCanComment(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	proposal, trade := f.item(t, feed.KindProposal), f.item(t, feed.KindTrade)
	for _, tt := range []struct {
		name   string
		item   uuid.UUID
		viewer ids.UserID
		want   bool
	}{
		{"member on a proposal", proposal, f.cabal.Members[1].ID, true},
		{"outsider on a proposal", proposal, f.outsider, false},
		{"outsider on a trade", trade, f.outsider, true},
	} {
		res, err := f.routes.GetFeedItem(asUser(t.Context(), tt.viewer), api.GetFeedItemRequestObject{Id: tt.item})
		got, ok := res.(api.GetFeedItem200JSONResponse)
		if err != nil || !ok || got.CanComment != tt.want {
			t.Errorf("%s: can_comment = %+v, %v, want %v", tt.name, res, err, tt.want)
		}
	}
}

func TestGetFeedItem_passesAMembershipFailureThrough(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	item := f.item(t, feed.KindProposal)
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE cabal_members CASCADE`); err != nil {
		t.Fatal(err)
	}
	_, err := f.routes.GetFeedItem(asUser(t.Context(), f.outsider), api.GetFeedItemRequestObject{Id: item})
	wantCode(t, err, errs.CodeInternal)
}

func TestComments_hintTheFeedThroughTheBusAfterACreateAndADelete(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	nats := testkit.NATS(t)
	seen := &hintLog{}
	if err := nats.Conn.SubscribeHints(t.Context(), func(ctx context.Context, key string) {
		seen.PublishHint(ctx, key, nil)
	}); err != nil {
		t.Fatal(err)
	}
	deps := module.Deps{Pool: f.pool, UoW: f.uow, IDs: f.gen, Clock: f.clock, Bus: nats.Conn}
	routes := social.HTTPOf(social.New(deps, social.WithUsers(fakes.NewIdentity(nil, nil))))
	ctx := asUser(t.Context(), f.outsider)
	res, err := routes.PostFeedComment(ctx, api.PostFeedCommentRequestObject{
		Id: f.item(t, feed.KindTrade), Body: &api.CreateCommentRequest{Body: "hi"},
	})
	if err != nil {
		t.Fatal(err)
	}
	created := api.Comment(res.(api.PostFeedComment201JSONResponse))
	if _, err := routes.DeleteFeedComment(ctx, api.DeleteFeedCommentRequestObject{CommentId: created.Id}); err != nil {
		t.Fatal(err)
	}
	testkit.Eventually(t, func() bool { return len(seen.seen()) == 2 }, 5*time.Second)
	if got := seen.seen(); !slices.Equal(got, []string{"global.feed", "global.feed"}) {
		t.Fatalf("hints = %v, want two global.feed", got)
	}
}
