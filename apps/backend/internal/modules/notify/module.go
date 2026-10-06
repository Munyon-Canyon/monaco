package notify

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/notifyapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Module struct {
	deps   module.Deps
	users  app.Users
	cabals app.Cabals
	assets app.Assets
	voters app.Voters
	sender apns.Sender
}

type Option func(*Module)

func WithUsers(users app.Users) Option {
	return func(m *Module) { m.users = users }
}

func WithCabals(cabals app.Cabals) Option {
	return func(m *Module) { m.cabals = cabals }
}

func WithAssets(assets app.Assets) Option {
	return func(m *Module) { m.assets = assets }
}

func WithVoters(voters app.Voters) Option {
	return func(m *Module) { m.voters = voters }
}

func WithSender(sender apns.Sender) Option {
	return func(m *Module) { m.sender = sender }
}

func New(d module.Deps, opts ...Option) *Module {
	m := &Module{
		deps: d, users: identity.New(d).Queries(), cabals: cabal.New(d).Queries(),
		assets: market.New(d).Catalog(), voters: governance.New(d).Voters(), sender: d.APNs,
	}
	for _, opt := range opts {
		opt(m)
	}
	if m.sender == nil {
		m.sender = apns.NoopSender{}
	}
	return m
}

func (*Module) Name() string { return "notify" }

func (m *Module) Mount(r api.Mount) {
	notifyapi.Mount(adapters.HTTP{
		Register:   app.NewRegisterDeviceHandler(m.deps.UoW, m.users, m.deps.IDs, m.deps.Clock),
		Unregister: app.NewUnregisterDeviceHandler(m.deps.UoW),
	}, r)
}

func (m *Module) pusher() *app.Pusher {
	return app.NewPusher(m.deps.UoW, m.users, m.sender, m.deps.IDs, m.deps.Clock)
}

func (m *Module) Consumers() []bus.Consumer {
	pusher := m.pusher()
	return []bus.Consumer{{Durable: "notify", Handlers: []bus.HandlerSpec{
		adapters.Push(pusher, app.Test{}),
		adapters.Push(pusher, app.DepositCredited{}),
		adapters.Push(pusher, app.CabalPaused{Cabals: m.cabals}),
		adapters.Push(pusher, app.CabalResumed{Cabals: m.cabals}),
		adapters.Push(pusher, app.TradeFilled{Cabals: m.cabals, Assets: m.assets}),
		adapters.Push(pusher, app.TradeFailed{Cabals: m.cabals, Assets: m.assets}),
		adapters.Push(pusher, app.ProposalCreated{Cabals: m.cabals, Users: m.users, Assets: m.assets}),
		adapters.Push(pusher, app.ProposalPassed{Cabals: m.cabals, Voters: m.voters, Assets: m.assets}),
		adapters.Push(pusher, app.NewFollower{Users: m.users}),
		adapters.Push(pusher, app.Nudge{Users: m.users}),
		adapters.Push(pusher, app.CommentReply{Users: m.users}),
		adapters.Push(
			pusher,
			app.ChatMention{Cabals: m.cabals, Users: m.users},
			app.ChatThreadReply{Cabals: m.cabals, Users: m.users},
		),
	}}}
}

func (m *Module) Pollers() []poller.Poller {
	return []poller.Poller{app.NewFollowDigest(m.pusher())}
}
