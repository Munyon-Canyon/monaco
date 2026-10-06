//go:build faultpoints

package ranking_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func TestFlow19_RunValuation_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	flows.F19RunValuationCrashBeforeCommit(flow19Scenario(t))
}
