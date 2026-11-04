package social_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func TestFlow22d_WriteChatMoment_OK(t *testing.T) {
	t.Parallel()
	flows.F22dWriteChatMomentOK(chatScenario(t))
}

func TestFlow22d_WriteChatMoment_InvalidInput(t *testing.T) {
	t.Parallel()
	flows.F22dWriteChatMomentInvalidInput(chatScenario(t))
}
