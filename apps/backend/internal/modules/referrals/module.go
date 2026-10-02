package referrals

import (
	"context"
	"crypto/rand"
	"io"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Module struct {
	deps    module.Deps
	entropy func(context.Context) io.Reader
}

type Option func(*Module)

func WithEntropy(entropy func(context.Context) io.Reader) Option {
	return func(m *Module) { m.entropy = entropy }
}

func New(d module.Deps, opts ...Option) *Module {
	m := &Module{deps: d, entropy: func(context.Context) io.Reader { return rand.Reader }}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (*Module) Name() string { return "referrals" }

func (m *Module) Routes(r *httpx.Routes) { r.ReferralsRoutes = adapters.HTTP{Codes: m.Resolver()} }

func (m *Module) Consumers() []bus.Consumer {
	return []bus.Consumer{
		{
			Durable: "referrals",
			Handlers: []bus.HandlerSpec{
				bus.Handle("referrals.mint_code", adapters.MintCode{Entropy: m.entropy}.Handle),
			},
		},
	}
}

func (*Module) Pollers() []poller.Poller { return nil }

func (m *Module) Resolver() app.Resolver {
	return app.Resolver{Reads: m.deps.Pool, Users: identity.New(m.deps).Queries()}
}
