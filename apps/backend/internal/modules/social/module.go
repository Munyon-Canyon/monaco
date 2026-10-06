package social

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters/ably"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Module struct {
	deps     module.Deps
	users    app.Users
	members  app.ChatMembers
	cabals   app.Cabals
	realtime app.Realtime
	assets   app.Assets
}

type Option func(*Module)

func WithUsers(users app.Users) Option {
	return func(m *Module) { m.users = users }
}

func WithCabals(cabals app.Cabals) Option {
	return func(m *Module) { m.cabals = cabals }
}

func WithAssets(catalog market.Catalog) Option {
	return func(m *Module) { m.assets = NewAssets(catalog) }
}

type CatalogAssets struct{ catalog market.Catalog }

func NewAssets(catalog market.Catalog) CatalogAssets { return CatalogAssets{catalog} }

func (c CatalogAssets) AssetByMint(ctx context.Context, raw string) (app.AssetCard, error) {
	mint, err := market.ParseMint(raw)
	if err != nil {
		return app.AssetCard{}, err
	}
	asset, err := c.catalog.AssetByMint(ctx, mint)
	return app.AssetCard{
		ID: asset.ID.UUID(), Symbol: asset.Symbol, Name: asset.DisplayName, Decimals: asset.Decimals,
	}, err
}

func WithRealtime(realtime app.Realtime) Option {
	return func(m *Module) { m.realtime = realtime }
}

func New(d module.Deps, opts ...Option) *Module {
	m := &Module{deps: d}
	for _, opt := range opts {
		opt(m)
	}
	if m.users == nil {
		m.users = identity.New(d).Queries()
	}
	if m.assets == nil {
		m.assets = NewAssets(market.New(d).Catalog())
	}
	m.useDefaultRealtime()
	queries := cabal.New(d).Queries()
	m.members = queries
	if m.cabals == nil {
		m.cabals = queries
	}
	return m
}

func (m *Module) useDefaultRealtime() {
	if m.realtime != nil {
		return
	}
	if m.deps.Config.Ably.APIKey == "" {
		m.realtime = ably.Noop{}
		return
	}
	client, err := ably.New(m.deps.Config, m.deps.HTTPClient)
	if err != nil {
		panic(err)
	}
	m.realtime = client
}

type FollowsPort = app.FollowsPort

func (m *Module) Follows() app.Follows { return app.NewFollows(m.deps.Pool) }

func (m *Module) FollowCounts() interface {
	Counts(context.Context, ids.UserID) (int, int, error)
	FollowedByMe(context.Context, ids.UserID, ids.UserID) (bool, error)
} {
	return m.Follows()
}

func (m *Module) FollowGraph() interface {
	FollowingIDs(context.Context, ids.UserID) ([]ids.UserID, error)
} {
	return m.Follows()
}

func (m *Module) Chat() interface {
	UnreadCounts(context.Context, ids.UserID, []ids.CabalID) (map[ids.CabalID]int, error)
} {
	return app.NewUnread(m.deps.Pool)
}

func (*Module) Name() string { return "social" }

func (m *Module) Mount(r api.Mount) { socialapi.Mount(m.http(), r) }

func (m *Module) http() adapters.HTTP {
	chat := app.ChatDeps{
		UoW: m.deps.UoW, Members: m.members, Users: m.users, IDs: m.deps.IDs, Clock: m.deps.Clock,
		Publish: app.NewChatPublisher(m.realtime, adapters.ChatWire(m.users)),
	}
	comments := app.CommentDeps{
		UoW: m.deps.UoW, Reads: m.deps.Pool, Members: m.members, IDs: m.deps.IDs, Clock: m.deps.Clock,
		Hints: m.deps.Bus,
	}
	return adapters.HTTP{
		Follow: app.NewFollowHandler(app.FollowDeps{
			UoW: m.deps.UoW, Users: m.users, IDs: m.deps.IDs, Clock: m.deps.Clock,
		}),
		Unfollow:      app.NewUnfollowHandler(m.deps.UoW, m.deps.Clock),
		Mute:          app.NewMuteHandler(m.deps.UoW, m.deps.Clock),
		Unmute:        app.NewUnmuteHandler(m.deps.UoW),
		PostChat:      app.NewPostChatMessageHandler(chat),
		DeleteChat:    app.NewDeleteChatMessageHandler(chat),
		MarkSeen:      app.NewMarkChatSeenHandler(chat),
		Token:         app.NewRealtimeTokenHandler(m.members, m.realtime),
		CreateComment: app.NewCreateCommentHandler(comments),
		DeleteComment: app.NewDeleteCommentHandler(comments),
		Match: app.NewMatchContactsHandler(app.MatchContactsDeps{
			UoW: m.deps.UoW, Users: m.users, Clock: m.deps.Clock,
		}),
		Members: m.members,
		Reads:   m.deps.Pool,
		Clock:   m.deps.Clock,
		Users:   m.users,
	}
}

func (m *Module) Consumers() []bus.Consumer {
	feed := adapters.Feed{
		Bus: m.deps.Bus, Users: m.users, Cabals: m.cabals, Assets: m.assets, IDs: m.deps.IDs, UoW: m.deps.UoW,
	}
	return []bus.Consumer{
		{
			Durable: "social_feed",
			Handlers: []bus.HandlerSpec{
				bus.HandleFetched("social.feed", feed.FetchCreated, feed.ApplyCreated),
				bus.HandleFetched("social.feed.joined", feed.FetchJoined, feed.ApplyJoined),
				bus.Handle("social.feed.left", feed.Left),
				bus.HandleFetched("social.feed.profile_updated", feed.FetchProfile, feed.ApplyProfile),
				bus.HandleFetched("social.feed.cabal_updated", feed.FetchCabal, feed.ApplyCabal),
				bus.HandleFetched("social.feed.proposal_created", feed.FetchProposal, feed.ApplyProposal),
				bus.Handle("social.feed.proposal_passed", feed.ProposalPassed),
				bus.Handle("social.feed.proposal_failed", feed.ProposalFailed),
				bus.Handle("social.feed.proposal_expired", feed.ProposalExpired),
				bus.Handle("social.feed.proposal_withdrawn", feed.ProposalWithdrawn),
				bus.Handle("social.feed.proposal_voided", feed.ProposalVoided),
				bus.Handle("social.feed.proposal_executed", feed.ProposalExecuted),
				bus.Handle("social.feed.proposal_blocked", feed.ProposalBlocked),
				bus.Handle("social.feed.trade_failed", feed.TradeFailed),
				bus.HandleFetched("social.feed.trade_confirmed", feed.FetchTrade, feed.ApplyTrade),
				bus.Handle("social.feed.price_moved", feed.PriceMoved),
			},
		},
		{
			Durable:  "social_chat_seen_cleanup",
			Handlers: []bus.HandlerSpec{bus.Handle("social.chat_seen_cleanup", adapters.ChatSeenCleanup{}.Handle)},
		},
	}
}

func (*Module) Pollers() []poller.Poller { return nil }
