package notify_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlow34_ReadCatchUp_OK(t *testing.T) {
	t.Parallel()
	flows.F34ReadCatchUpOK(flow34Scenario(t))
}

func TestFlow34_ReadCatchUp_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F34ReadCatchUpUnauthorized(flow34Scenario(t))
}

func flow34Scenario(t *testing.T, opts ...scenario.Option) *scenario.Scenario {
	t.Helper()
	opts = append(opts, scenario.WithModules(func(d module.Deps) module.Module { return notify.New(d) }))
	return scenario.New(t, opts...)
}
