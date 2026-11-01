package app

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/app/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/app/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/appapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Module struct {
	deps module.Deps
}

func New(d module.Deps) *Module { return &Module{deps: d} }

func (*Module) Name() string { return "app" }

func (m *Module) Mount(r api.Mount) {
	minBuild, storeURL := m.deps.Config.App.UpgradeGate()
	appapi.Mount(adapters.HTTP{
		Config: app.NewGetConfig(app.Config{MinIOSBuild: minBuild, StoreURL: storeURL}),
	}, r)
}

func (*Module) Consumers() []bus.Consumer {
	return []bus.Consumer{}
}

func (*Module) Pollers() []poller.Poller { return nil }
