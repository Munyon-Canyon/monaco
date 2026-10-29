package notify_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlow35_ClearItem_OK(t *testing.T) {
	t.Parallel()
	flows.F35ClearItemOK(flow35Scenario(t))
}

func TestFlow35_ClearItem_NotFound(t *testing.T) {
	t.Parallel()
	flows.F35ClearItemNotFound(flow35Scenario(t))
}

func TestFlow35_ClearItem_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F35ClearItemUnauthorized(flow35Scenario(t))
}

func flow35Scenario(t *testing.T, opts ...scenario.Option) *scenario.Scenario {
	t.Helper()
	opts = append(opts, scenario.WithModules(func(d module.Deps) module.Module { return notify.New(d) }))
	return scenario.New(t, opts...)
}
