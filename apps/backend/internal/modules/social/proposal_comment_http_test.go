package social_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/feedtest"
)

func (f commentRoutes) proposalItem(t *testing.T) (proposal, item uuid.UUID) {
	t.Helper()
	proposal = f.gen.NewV7()
	item = feedtest.Item(t, f.pool, f.clock, f.gen, func(it *feed.Item) {
		it.Kind, it.RefID, it.CabalID, it.ActorID = feed.KindProposal, proposal, f.cabal.ID, f.cabal.Creator.ID
		it.Payload = feed.Payload{CabalName: "Alpha Cabal", Symbol: "AAPLx", AssetName: "Apple", Action: feed.ActionBuy}
	})
	return proposal, item
}

func (f commentRoutes) postProposal(
	t *testing.T, proposal uuid.UUID, author ids.UserID, body string,
) (api.Comment, error) {
	t.Helper()
	res, err := f.routes.PostProposalComment(asUser(t.Context(), author),
		api.PostProposalCommentRequestObject{Id: proposal, Body: &api.CreateCommentRequest{Body: body}})
	if err != nil {
		return api.Comment{}, err
	}
	return api.Comment(res.(api.PostProposalComment201JSONResponse)), nil
}

func TestProposalComments_Resolve(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	kai := f.cabal.Members[0].ID
	proposal, item := f.proposalItem(t)

	posted, err := f.postProposal(t, proposal, kai, "agree")
	if err != nil {
		t.Fatal(err)
	}

	limit := 1
	res, err := f.routes.GetProposalComments(asUser(t.Context(), kai),
		api.GetProposalCommentsRequestObject{Id: proposal, Params: api.GetProposalCommentsParams{Limit: &limit}})
	if err != nil {
		t.Fatal(err)
	}
	page := res.(api.GetProposalComments200JSONResponse)
	if page.FeedObjectId != item || len(page.Items) != 1 || page.Items[0].Comment.Id != posted.Id {
		t.Fatalf("page = %+v, want comment %s on item %s", page, posted.Id, item)
	}
	if stored, counted := f.count(t, item); stored != 1 || counted != 1 {
		t.Fatalf("stored %d, counted %d, want 1 and 1", stored, counted)
	}

	feedRes, err := f.routes.GetFeedComments(asUser(t.Context(), kai), api.GetFeedCommentsRequestObject{Id: item})
	if err != nil {
		t.Fatal(err)
	}
	got := feedRes.(api.GetFeedComments200JSONResponse)
	if len(got.Items) != 1 || got.Items[0].Comment.Id != posted.Id {
		t.Fatalf("feed route = %+v, want the proposal comment", got)
	}
}

func TestProposalComments_Pending(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	kai := f.cabal.Members[0].ID
	missing := f.gen.NewV7()

	_, err := f.postProposal(t, missing, kai, "early")
	wantCode(t, err, errs.CodeFeedItemPending)
	_, err = f.routes.GetProposalComments(asUser(t.Context(), kai), api.GetProposalCommentsRequestObject{Id: missing})
	wantCode(t, err, errs.CodeFeedItemPending)

	proposal, _ := f.proposalItem(t)
	if _, err := f.postProposal(t, proposal, kai, "now"); err != nil {
		t.Fatal(err)
	}
}

func TestProposalComments_MembersOnly(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	proposal, _ := f.proposalItem(t)

	_, err := f.postProposal(t, proposal, f.outsider, "hi")
	wantCode(t, err, errs.CodeCommentMembersOnly)
}

func TestProposalComments_RefuseWhatTheFeedRoutesRefuse(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	kai := f.cabal.Members[0].ID
	proposal, _ := f.proposalItem(t)
	badCursor := "not-a-cursor"

	_, err := f.postProposal(t, proposal, kai, " ")
	wantCode(t, err, errs.CodeInvalidInput)
	_, err = f.routes.GetProposalComments(asUser(t.Context(), kai), api.GetProposalCommentsRequestObject{
		Id: proposal, Params: api.GetProposalCommentsParams{Cursor: &badCursor},
	})
	wantCode(t, err, errs.CodeInvalidInput)
}

func TestProposalComments_ALookupFailureIsInternal(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	kai := f.cabal.Members[0].ID
	proposal, _ := f.proposalItem(t)
	ctx, cancel := context.WithCancel(asUser(t.Context(), kai))
	cancel()

	_, err := f.routes.GetProposalComments(ctx, api.GetProposalCommentsRequestObject{Id: proposal})
	wantCode(t, err, errs.CodeInternal)
	_, err = f.routes.PostProposalComment(ctx, api.PostProposalCommentRequestObject{
		Id: proposal, Body: &api.CreateCommentRequest{Body: "x"},
	})
	wantCode(t, err, errs.CodeInternal)
}
