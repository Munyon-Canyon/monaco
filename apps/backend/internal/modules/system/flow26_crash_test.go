//go:build faultpoints

package system_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlow26_FlagPing_CrashAfterPublish(t *testing.T) {
	t.Parallel()
	flows.F26FlagPingCrashAfterPublish(scenario.New(t, withSystemAndAdmin()))
}
