package ranking

import (
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Module struct{ ports app.Ports }

type Option func(*Module)

func WithPorts(ports app.Ports) Option { return func(m *Module) { m.ports = ports } }

func New(_ module.Deps, opts ...Option) *Module {
	m := &Module{}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *Module) Wire(set module.Set) {
	for _, mod := range set {
		switch provider := mod.(type) {
		case interface{ Queries() treasuryport.Queries }:
			m.ports.Treasury = provider.Queries()
		case interface{ Pauses() fundingport.Pauses }:
			m.ports.Funding = provider.Pauses()
		case interface{ Queries() cabalport.Queries }:
			m.ports.Cabals = provider.Queries()
		case interface{ Queries() identityport.Queries }:
			m.ports.Users = provider.Queries()
		}
	}
}

func (m *Module) Ports() app.Ports { return m.ports }

func (*Module) Name() string { return "ranking" }

func (*Module) Mount(api.Mount) {}

func (*Module) Consumers() []bus.Consumer {
	return []bus.Consumer{}
}

func (*Module) Pollers() []poller.Poller { return nil }
