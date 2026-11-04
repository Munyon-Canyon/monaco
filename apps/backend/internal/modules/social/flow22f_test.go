package social_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func TestFlow22f_PinChatMessage_OK(t *testing.T) {
	t.Parallel()
	flows.F22fPinChatMessageOK(chatScenario(t))
}

func TestFlow22f_PinChatMessage_NotCabalMember(t *testing.T) {
	t.Parallel()
	flows.F22fPinChatMessageNotCabalMember(chatScenario(t))
}

func TestFlow22f_PinChatMessage_ChatMessageNotFound(t *testing.T) {
	t.Parallel()
	flows.F22fPinChatMessageChatMessageNotFound(chatScenario(t))
}

func TestFlow22f_PinChatMessage_ChatPinForbidden(t *testing.T) {
	t.Parallel()
	flows.F22fPinChatMessageChatPinForbidden(chatScenario(t))
}

func TestFlow22f_PinChatMessage_ChatPinLimit(t *testing.T) {
	t.Parallel()
	flows.F22fPinChatMessageChatPinLimit(chatScenario(t))
}

func TestFlow22f_PinChatMessage_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F22fPinChatMessageUnauthorized(chatScenario(t))
}

func TestFlow22f_UnpinChatMessage_OK(t *testing.T) {
	t.Parallel()
	flows.F22fUnpinChatMessageOK(chatScenario(t))
}

func TestFlow22f_UnpinChatMessage_NotCabalMember(t *testing.T) {
	t.Parallel()
	flows.F22fUnpinChatMessageNotCabalMember(chatScenario(t))
}

func TestFlow22f_UnpinChatMessage_ChatMessageNotFound(t *testing.T) {
	t.Parallel()
	flows.F22fUnpinChatMessageChatMessageNotFound(chatScenario(t))
}

func TestFlow22f_UnpinChatMessage_ChatPinForbidden(t *testing.T) {
	t.Parallel()
	flows.F22fUnpinChatMessageChatPinForbidden(chatScenario(t))
}
