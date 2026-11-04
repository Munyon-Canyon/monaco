package social_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func TestFlow22a_ReactChatMessage_OK(t *testing.T) {
	t.Parallel()
	flows.F22aReactChatMessageOK(chatScenario(t))
}

func TestFlow22a_ReactChatMessage_InvalidInput(t *testing.T) {
	t.Parallel()
	flows.F22aReactChatMessageInvalidInput(chatScenario(t))
}

func TestFlow22a_ReactChatMessage_NotCabalMember(t *testing.T) {
	t.Parallel()
	flows.F22aReactChatMessageNotCabalMember(chatScenario(t))
}

func TestFlow22a_ReactChatMessage_ChatMessageNotFound(t *testing.T) {
	t.Parallel()
	flows.F22aReactChatMessageChatMessageNotFound(chatScenario(t))
}

func TestFlow22a_ReactChatMessage_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F22aReactChatMessageUnauthorized(chatScenario(t))
}

func TestFlow22a_UnreactChatMessage_OK(t *testing.T) {
	t.Parallel()
	flows.F22aUnreactChatMessageOK(chatScenario(t))
}

func TestFlow22a_UnreactChatMessage_InvalidInput(t *testing.T) {
	t.Parallel()
	flows.F22aUnreactChatMessageInvalidInput(chatScenario(t))
}

func TestFlow22a_UnreactChatMessage_NotCabalMember(t *testing.T) {
	t.Parallel()
	flows.F22aUnreactChatMessageNotCabalMember(chatScenario(t))
}

func TestFlow22a_UnreactChatMessage_ChatMessageNotFound(t *testing.T) {
	t.Parallel()
	flows.F22aUnreactChatMessageChatMessageNotFound(chatScenario(t))
}

func TestFlow22a_UnreactChatMessage_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F22aUnreactChatMessageUnauthorized(chatScenario(t))
}
