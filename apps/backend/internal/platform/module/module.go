package module

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/sse"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Deps struct {
	Config     config.Config
	Logger     *slog.Logger
	Clock      clock.Clock
	IDs        ids.Generator
	Pool       *pgxpool.Pool
	UoW        *db.UnitOfWork
	Bus        *bus.Conn
	HTTPClient func(name string, opts ...httpclient.Option) *httpclient.Client
	Hub        *sse.Hub
	APNs       apns.Sender
}

type Module interface {
	Name() string
	Routes(r *httpx.Routes)
	Consumers() []bus.Consumer
	Pollers() []poller.Poller
}

type Registry struct {
	builders []func(Deps) Module
}

func (r *Registry) Add(build func(Deps) Module) {
	r.builders = append(r.builders, build)
}

func (r *Registry) Build(d Deps) Set {
	mods := make([]Module, 0, 1+len(r.builders))
	mods = append(mods, platform{d: d})
	for _, build := range r.builders {
		mods = append(mods, build(d))
	}
	return NewSet(mods...)
}

type Set []Module

func NewSet(mods ...Module) Set {
	seen := map[string]bool{}
	for _, m := range mods {
		if seen[m.Name()] {
			panic("module: " + m.Name() + " registered twice")
		}
		seen[m.Name()] = true
	}
	return mods
}

func (s Set) Routes() httpx.Routes {
	var r httpx.Routes
	for _, m := range s {
		m.Routes(&r)
	}
	return r
}

func (s Set) Consumers() []bus.Consumer {
	var out []bus.Consumer
	for _, m := range s {
		out = append(out, m.Consumers()...)
	}
	return out
}

func (s Set) Pollers() []poller.Poller {
	var out []poller.Poller
	for _, m := range s {
		out = append(out, m.Pollers()...)
	}
	return out
}

type platform struct {
	d Deps
}

func (platform) Name() string { return "platform" }

func (p platform) Routes(r *httpx.Routes) { r.Stream = sse.NewStream(p.d.Hub, p.d.Clock) }

func (platform) Consumers() []bus.Consumer { return nil }

func (p platform) Pollers() []poller.Poller {
	return []poller.Poller{poller.NewRetention(p.d.Pool, p.d.Clock)}
}
