package social_test

import (
	"testing"

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
