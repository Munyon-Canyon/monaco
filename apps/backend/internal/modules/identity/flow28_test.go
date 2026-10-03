package identity_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func TestFlow28_EmitNudges_OK(t *testing.T) {
	t.Parallel()
	flows.F28EmitNudgesOK(identityScenario(t))
}
