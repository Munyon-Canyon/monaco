package notify_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func TestFlow33_RemindVoters_OK(t *testing.T) {
	t.Parallel()
	flows.F33RemindVotersOK(flow24Scenario(t))
}
