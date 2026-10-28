//go:build faultpoints

package notify_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func TestFlow33_RemindVoters_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	flows.F33RemindVotersCrashBeforeCommit(flow24Scenario(t))
}
