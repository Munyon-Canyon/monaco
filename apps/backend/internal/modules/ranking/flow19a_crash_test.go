//go:build faultpoints

package ranking_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func TestFlow19a_ResolveCalls_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	flows.F19aResolveCallsCrashBeforeCommit(flow19Scenario(t))
}
