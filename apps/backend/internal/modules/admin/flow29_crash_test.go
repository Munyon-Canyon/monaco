//go:build faultpoints

package admin_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func TestFlow29_ApproveCabalBan_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	flows.F29ApproveCabalBanCrashBeforeCommit(flow29(t))
}
