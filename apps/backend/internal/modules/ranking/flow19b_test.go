package ranking_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func TestFlow19b_RunCallBoard_OK(t *testing.T) {
	t.Parallel()
	flows.F19bRunCallBoardOK(flow19Scenario(t))
}
