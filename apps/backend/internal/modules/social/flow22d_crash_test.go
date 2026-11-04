//go:build faultpoints

package social_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func TestFlow22d_WriteMemberJoinedMoment_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	flows.F22dWriteMemberJoinedMomentCrashBeforeCommit(chatScenario(t))
}
