package system_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/admin"
	"github.com/monaco/monaco/apps/backend/internal/modules/system"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func withSystemAndAdmin() scenario.Option {
	return scenario.WithModules(
		func(d module.Deps) module.Module { return system.New(d) },
		func(d module.Deps) module.Module { return admin.New(d) },
	)
}

func TestFlow26_FlagPing_OK(t *testing.T) {
	t.Parallel()
	flows.F26FlagPingOK(scenario.New(t, withSystemAndAdmin()))
}

func TestFlow26_FlagPing_ReasonRequired(t *testing.T) {
	t.Parallel()
	flows.F26FlagPingReasonRequired(scenario.New(t, withSystemAndAdmin()))
}

func TestFlow26_FlagPing_AdminForbidden(t *testing.T) {
	t.Parallel()
	flows.F26FlagPingAdminForbidden(scenario.New(t, withSystemAndAdmin()))
}

func TestFlow26_FlagPing_AlreadyFlagged(t *testing.T) {
	t.Parallel()
	flows.F26FlagPingAlreadyFlagged(scenario.New(t, withSystemAndAdmin()))
}

func TestFlow26_FlagPing_NotFound(t *testing.T) {
	t.Parallel()
	flows.F26FlagPingNotFound(scenario.New(t, withSystemAndAdmin()))
}
