package notify_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlow32_SetNotificationPreferences_OK(t *testing.T) {
	t.Parallel()
	flows.F32SetNotificationPreferencesOK(flow32Scenario(t))
}

func TestFlow32_SetNotificationPreferences_InvalidInput(t *testing.T) {
	t.Parallel()
	flows.F32SetNotificationPreferencesInvalidInput(flow32Scenario(t))
}

func TestFlow32_SetNotificationPreferences_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F32SetNotificationPreferencesUnauthorized(flow32Scenario(t))
}

func TestFlow32_SetNotificationPreferences_RateLimited(t *testing.T) {
	t.Parallel()
	flows.F32SetNotificationPreferencesRateLimited(flow32Scenario(t))
}

func flow32Scenario(t *testing.T, opts ...scenario.Option) *scenario.Scenario {
	t.Helper()
	opts = append(opts, scenario.WithModules(func(d module.Deps) module.Module { return notify.New(d) }))
	return scenario.New(t, opts...)
}
