package social_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlow21_CreateComment_OK(t *testing.T) {
	t.Parallel()
	flows.F21CreateCommentOK(scenario.New(t, withSocial(), scenario.WithPostHog(t)))
}

func TestFlow21_CreateComment_InvalidInput(t *testing.T) {
	t.Parallel()
	flows.F21CreateCommentInvalidInput(scenario.New(t, withSocial()))
}

func TestFlow21_CreateComment_FeedItemNotFound(t *testing.T) {
	t.Parallel()
	flows.F21CreateCommentFeedItemNotFound(scenario.New(t, withSocial()))
}

func TestFlow21_CreateComment_CommentParentMismatch(t *testing.T) {
	t.Parallel()
	flows.F21CreateCommentCommentParentMismatch(scenario.New(t, withSocial()))
}

func TestFlow21_CreateComment_CommentMembersOnly(t *testing.T) {
	t.Parallel()
	flows.F21CreateCommentCommentMembersOnly(scenario.New(t, withSocial()))
}

func TestFlow21_CreateComment_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F21CreateCommentUnauthorized(scenario.New(t, withSocial()))
}

func TestFlow21_CreateComment_RateLimited(t *testing.T) {
	t.Parallel()
	flows.F21CreateCommentRateLimited(scenario.New(t, withSocial()))
}

func TestCreateComment_aResultOrThresholdSharesItsProposalsThreadOnRead(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	member := f.cabal.Members[0].ID
	proposal := f.item(t, feed.KindProposal)
	ref := f.proposalRef(t, proposal)
	result := f.closing(t, feed.KindResult, feed.ShapeCabal, ref)
	threshold := f.closing(t, feed.KindThreshold, "", ref)
	f.say(t, result, member, "on the result", nil)
	f.say(t, threshold, member, "on the threshold", nil)
	for _, card := range []uuid.UUID{result, threshold, proposal} {
		page := f.read(t, f.outsider, card, api.GetFeedCommentsParams{})
		if len(page.Items) != 2 || page.Items[0].Comment.BodyDisplay != "on the result" {
			t.Errorf("comments read through %s = %+v, want the proposal's two", card, page.Items)
		}
	}
	if stored, counted := f.count(t, proposal); stored != 2 || counted != 2 {
		t.Fatalf("proposal thread: stored %d, comment_count %d, want 2 and 2", stored, counted)
	}
}
