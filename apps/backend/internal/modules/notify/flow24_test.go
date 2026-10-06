package notify_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlow24_Notify_OK(t *testing.T) {
	t.Parallel()
	flows.F24NotifyOK(scenario.New(t, scenario.WithModules(func(d module.Deps) module.Module { return notify.New(d) })))
}
