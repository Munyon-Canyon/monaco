package social_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func TestFlow22e_WriteChatProposalMessage_OK(t *testing.T) {
	t.Parallel()
	flows.F22eWriteChatProposalMessageOK(chatScenario(t))
}

func TestFlow22e_WriteChatProposalMessage_ProposalNotFound(t *testing.T) {
	t.Parallel()
	flows.F22eWriteChatProposalMessageProposalNotFound(chatScenario(t))
}
