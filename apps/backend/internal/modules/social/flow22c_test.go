package social_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func TestFlow22c_VoteChatPoll_OK(t *testing.T) {
	t.Parallel()
	flows.F22cVoteChatPollOK(chatScenario(t))
}

func TestFlow22c_VoteChatPoll_InvalidInput(t *testing.T) {
	t.Parallel()
	flows.F22cVoteChatPollInvalidInput(chatScenario(t))
}

func TestFlow22c_VoteChatPoll_NotCabalMember(t *testing.T) {
	t.Parallel()
	flows.F22cVoteChatPollNotCabalMember(chatScenario(t))
}

func TestFlow22c_VoteChatPoll_ChatMessageNotFound(t *testing.T) {
	t.Parallel()
	flows.F22cVoteChatPollChatMessageNotFound(chatScenario(t))
}

func TestFlow22c_VoteChatPoll_PollClosed(t *testing.T) {
	t.Parallel()
	flows.F22cVoteChatPollPollClosed(chatScenario(t))
}

func TestFlow22c_VoteChatPoll_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F22cVoteChatPollUnauthorized(chatScenario(t))
}

func TestFlow22c_UnvoteChatPoll_OK(t *testing.T) {
	t.Parallel()
	flows.F22cUnvoteChatPollOK(chatScenario(t))
}

func TestFlow22c_UnvoteChatPoll_NotCabalMember(t *testing.T) {
	t.Parallel()
	flows.F22cUnvoteChatPollNotCabalMember(chatScenario(t))
}

func TestFlow22c_UnvoteChatPoll_ChatMessageNotFound(t *testing.T) {
	t.Parallel()
	flows.F22cUnvoteChatPollChatMessageNotFound(chatScenario(t))
}

func TestFlow22c_UnvoteChatPoll_PollClosed(t *testing.T) {
	t.Parallel()
	flows.F22cUnvoteChatPollPollClosed(chatScenario(t))
}
