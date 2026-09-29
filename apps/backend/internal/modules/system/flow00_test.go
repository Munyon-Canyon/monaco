package system_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlow00_RecordPing_OK(t *testing.T) {
	t.Parallel()
	flows.F00RecordPingOK(scenario.New(t, withSystem()))
}

func TestFlow00_RecordPing_InvalidInput(t *testing.T) {
	t.Parallel()
	flows.F00RecordPingInvalidInput(scenario.New(t, withSystem()))
}

func TestFlow00_RecordPing_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F00RecordPingUnauthorized(scenario.New(t, withSystem()))
}
