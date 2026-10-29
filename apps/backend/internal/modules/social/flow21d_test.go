package social_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlow21d_VoteCommentPoll_OK(t *testing.T) {
	t.Parallel()
	flows.F21dVoteCommentPollOK(scenario.New(t, withSocial()))
}

func TestFlow21d_VoteCommentPoll_InvalidInput(t *testing.T) {
	t.Parallel()
	flows.F21dVoteCommentPollInvalidInput(scenario.New(t, withSocial()))
}

func TestFlow21d_VoteCommentPoll_PollClosed(t *testing.T) {
	t.Parallel()
	flows.F21dVoteCommentPollPollClosed(scenario.New(t, withSocial()))
}

func TestFlow21d_VoteCommentPoll_CommentNotFound(t *testing.T) {
	t.Parallel()
	flows.F21dVoteCommentPollCommentNotFound(scenario.New(t, withSocial()))
}

func TestFlow21d_VoteCommentPoll_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F21dVoteCommentPollUnauthorized(scenario.New(t, withSocial()))
}

func TestFlow21d_VoteCommentPoll_RateLimited(t *testing.T) {
	t.Parallel()
	flows.F21dVoteCommentPollRateLimited(scenario.New(t, withSocial()))
}
