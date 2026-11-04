package social_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func TestFlow22d_WriteMemberJoinedMoment_OK(t *testing.T) {
	t.Parallel()
	flows.F22dWriteMemberJoinedMomentOK(chatScenario(t))
}

func TestFlow22d_WriteCabalCreatedMoment_OK(t *testing.T) {
	t.Parallel()
	flows.F22dWriteCabalCreatedMomentOK(chatScenario(t))
}

func TestFlow22d_WriteTradeFilledMoment_OK(t *testing.T) {
	t.Parallel()
	flows.F22dWriteTradeFilledMomentOK(chatScenario(t))
}
