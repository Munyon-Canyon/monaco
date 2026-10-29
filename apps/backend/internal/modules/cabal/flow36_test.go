package cabal_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func TestFlow36_ReorderCabals_OK(t *testing.T) {
	t.Parallel()
	flows.F36ReorderCabalsOK(cabalScenario(t))
}

func TestFlow36_ReorderCabals_InvalidInput(t *testing.T) {
	t.Parallel()
	flows.F36ReorderCabalsInvalidInput(cabalScenario(t))
}

func TestFlow36_ReorderCabals_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F36ReorderCabalsUnauthorized(cabalScenario(t))
}
