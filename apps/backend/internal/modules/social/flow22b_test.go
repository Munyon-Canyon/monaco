package social_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func TestFlow22b_PostChatMessage_OK(t *testing.T) {
	t.Parallel()
	flows.F22bPostChatMessageOK(chatScenario(t))
}

func TestFlow22b_PostChatMessage_InvalidInput(t *testing.T) {
	t.Parallel()
	flows.F22bPostChatMessageInvalidInput(chatScenario(t))
}

func TestFlow22b_PostChatMessage_NotCabalMember(t *testing.T) {
	t.Parallel()
	flows.F22bPostChatMessageNotCabalMember(chatScenario(t))
}

func TestFlow22b_PostChatMessage_ChatBodyInvalid(t *testing.T) {
	t.Parallel()
	flows.F22bPostChatMessageChatBodyInvalid(chatScenario(t))
}

func TestFlow22b_PostChatMessage_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F22bPostChatMessageUnauthorized(chatScenario(t))
}
