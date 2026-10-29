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
	deps    module.Deps
	users   app.Users
	cabals  app.Cabals
	assets  app.Assets
	ballots app.Ballots
	heads   app.ProposalHeads
	votes   app.VoteStates
	sender  apns.Sender
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

func WithBallots(ballots app.Ballots) Option {
	return func(m *Module) { m.ballots = ballots }
}

func WithProposalHeads(heads app.ProposalHeads) Option {
	return func(m *Module) { m.heads = heads }
}

func WithVoteStates(votes app.VoteStates) Option {
	return func(m *Module) { m.votes = votes }
}

func WithSender(sender apns.Sender) Option {
	return func(m *Module) { m.sender = sender }
}

func New(d module.Deps, opts ...Option) *Module {
	gov := governance.New(d)
	m := &Module{
		deps:   d,
		users:  identity.New(d).Queries(),
		cabals: cabal.New(d).Queries(),
		assets: market.New(d).
			Catalog(),
		ballots: gov.Ballots(),
		heads:   gov.ProposalHeads(),
		votes:   gov.VoteStates(),
		sender:  d.APNs,
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
		SetPrefs:   app.NewSetNotificationPreferencesHandler(m.deps.UoW, m.users, m.deps.Clock),
		Reads:      m.deps.Pool,
		Users:      m.users,
	}, r)
}

func (m *Module) pusher() *app.Pusher {
	return app.NewPusher(m.deps.UoW, m.users, m.sender, m.deps.IDs, m.deps.Clock)
}

func (m *Module) Consumers() []bus.Consumer {
	pusher := m.pusher()
	results := app.ResultPorts{Cabals: m.cabals, Assets: m.assets, Ballots: m.ballots, Heads: m.heads}
	return []bus.Consumer{{Durable: "notify", Handlers: []bus.HandlerSpec{
		adapters.Push(pusher, app.Test{}),
		adapters.Push(pusher, app.DepositCredited{}),
		adapters.Push(pusher, app.CabalPaused{Cabals: m.cabals}),
		adapters.Push(pusher, app.CabalResumed{Cabals: m.cabals}),
		adapters.Push(pusher, app.ResultFilled{ResultPorts: results}),
		adapters.Push(pusher, app.ResultFillFailed{ResultPorts: results}),
		adapters.Push(pusher, app.ResultFillBlocked{ResultPorts: results}),
		adapters.Push(pusher, app.ProposalCreated{
			Cabals: m.cabals, Users: m.users, Assets: m.assets, Ballots: m.ballots, Heads: m.heads, Clock: m.deps.Clock,
		}),
		adapters.Push(pusher, app.VoteDeciding{VotePorts: m.votePorts(), Clock: m.deps.Clock}),
		adapters.Push(pusher, app.ResultFailed{ResultPorts: results}),
		adapters.Push(pusher, app.ResultExpired{ResultPorts: results}),
		adapters.Push(pusher, app.CabalAccessRequestedAdmins{
			CabalAccessRequested: app.CabalAccessRequested{Cabals: m.cabals, Users: m.users}, Admins: m.cabals,
		}),
		adapters.Push(pusher, app.CabalAccessApproved{Cabals: m.cabals}),
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

func (m *Module) votePorts() app.VotePorts {
	return app.VotePorts{Cabals: m.cabals, Assets: m.assets, States: m.votes}
}

func (m *Module) Pollers() []poller.Poller {
	pusher := m.pusher()
	return []poller.Poller{app.NewFollowDigest(pusher), app.NewRemindVoters(pusher, m.votePorts())}
}
