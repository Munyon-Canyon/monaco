package analytics

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/adapters/posthog"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Module struct {
	deps    module.Deps
	exports *Registry
}

func New(d module.Deps) *Module { return newModule(d, productExports()) }

func newModule(d module.Deps, r *Registry) *Module { return &Module{deps: d, exports: r} }

func productExports() *Registry { return NewRegistry() }

func (*Module) Name() string { return "analytics" }

func (*Module) Mount(api.Mount) {}

func (m *Module) Consumers() []bus.Consumer {
	var port app.PostHog = posthog.Noop{}
	if m.deps.Config.PostHog.APIKey != "" {
		port = posthog.New(m.deps.Config, m.deps.HTTPClient)
	}
	if c, ok := m.exports.Consumer(port); ok {
		return []bus.Consumer{c}
	}
	return nil
}

func (*Module) Pollers() []poller.Poller { return nil }
