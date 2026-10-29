package app_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func withApp(cfg config.App) scenario.Option {
	return scenario.WithModules(func(d module.Deps) module.Module {
		d.Config.App = cfg
		return app.New(d)
	})
}

func TestFlow39_GetConfig_OK(t *testing.T) {
	t.Parallel()
	flows.F39GetConfigOK(scenario.New(t, withApp(config.App{StoreURL: "https://testflight.apple.com/join/monaco"})))
}

func TestModule_servesTheConfigWithTheGateOn(t *testing.T) {
	t.Parallel()
	m := app.New(module.Deps{})
	if m.Name() != "app" || !testkit.Serves(m.Mount, "GET", "/v1/app/config") || m.Pollers() != nil ||
		len(m.Consumers()) != 0 {
		t.Fatalf("module = %s, %d consumers", m.Name(), len(m.Consumers()))
	}
}

func TestGetConfig_returnsTheConfiguredMinimumBuild(t *testing.T) {
	t.Parallel()
	s := scenario.New(t, withApp(config.App{MinIOSBuild: 120, StoreURL: "https://testflight.apple.com/join/example"}))
	s.Given(scenario.Anonymous()).
		When(
			scenario.Get("/v1/app/config"),
			scenario.ExpectJSON("min_ios_build", 120),
			scenario.ExpectJSON("store_url", "https://testflight.apple.com/join/example"),
		)
}
