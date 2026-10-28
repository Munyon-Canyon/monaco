package social

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters/ably"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
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
	returns  app.Returns
	members  app.ChatMembers
	cabals   app.Cabals
	realtime app.Realtime
	assets   app.Assets
	logos    app.AssetLogos
	pictures app.CabalPictures
	treasury app.Treasury
}

type Option func(*Module)

func WithUsers(users app.Users) Option {
	return func(m *Module) { m.users = users }
}

func WithReturns(returns app.Returns) Option {
	return func(m *Module) { m.returns = returns }
}

func WithCabals(cabals app.Cabals) Option {
	return func(m *Module) { m.cabals = cabals }
}

func WithCabalPictures(pictures app.CabalPictures) Option {
	return func(m *Module) { m.pictures = pictures }
}

func WithTreasury(treasury app.Treasury) Option {
	return func(m *Module) { m.treasury = treasury }
}

func WithAssets(catalog market.Catalog) Option {
	return func(m *Module) {
		assets := NewAssets(catalog)
		m.assets, m.logos = assets, assets
	}
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

func (c CatalogAssets) AssetLogos(ctx context.Context, assetIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	logos := make(map[uuid.UUID]string, len(assetIDs))
	for _, id := range assetIDs {
		asset, err := c.catalog.AssetByID(ctx, market.AssetIDFrom(id))
		switch {
		case errs.CodeOf(err) == errs.CodeAssetNotFound:
			continue
		case err != nil:
			return nil, err
		}
		logos[id] = asset.LogoURL
	}
	return logos, nil
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
	if m.returns == nil {
		m.returns = ranking.New(d).Queries()
	}
	if m.assets == nil {
		assets := NewAssets(market.New(d).Catalog())
		m.assets, m.logos = assets, assets
	}
	if m.treasury == nil {
		m.treasury = treasury.New(d).Queries()
	}
	m.useDefaultRealtime()
	queries := cabal.New(d).Queries()
	m.members = queries
	if m.cabals == nil {
		m.cabals = queries
	}
	if m.pictures == nil {
		m.pictures = queries
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

func (m *Module) ReplyCounts(ctx context.Context, proposalIDs []ids.ProposalID) (map[ids.ProposalID]int, error) {
	return app.ReplyCounts(ctx, m.deps.Pool, proposalIDs)
}

func (m *Module) Follows() app.Follows { return app.NewFollows(m.deps.Pool) }

func (m *Module) FollowCounts() interface {
	Counts(context.Context, ids.UserID) (int, int, error)
	FollowedByMe(context.Context, ids.UserID, ids.UserID) (bool, error)
	FollowedAmong(context.Context, ids.UserID, []ids.UserID) (map[ids.UserID]bool, error)
	BlockedByMe(context.Context, ids.UserID, ids.UserID) (bool, error)
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
		UoW: m.deps.UoW, Reads: m.deps.Pool, Members: m.members, Users: m.users, IDs: m.deps.IDs,
		Clock: m.deps.Clock, Hints: m.deps.Bus,
	}
	likes := app.LikeDeps{UoW: m.deps.UoW, Reads: m.deps.Pool, Clock: m.deps.Clock, Hints: m.deps.Bus}
	return adapters.HTTP{
		Follow: app.NewFollowHandler(app.FollowDeps{
			UoW: m.deps.UoW, Users: m.users, IDs: m.deps.IDs, Clock: m.deps.Clock,
		}),
		Unfollow: app.NewUnfollowHandler(m.deps.UoW, m.deps.Clock),
		Block: app.NewBlockUserHandler(app.BlockUserDeps{
			UoW: m.deps.UoW, Users: m.users, IDs: m.deps.IDs, Clock: m.deps.Clock,
		}),
		Unblock: app.NewUnblockUserHandler(m.deps.UoW, m.deps.Clock),
		Report: app.NewCreateReportHandler(app.CreateReportDeps{
			UoW: m.deps.UoW, Reads: m.deps.Pool, Users: m.users, Cabals: m.cabals, Members: m.members,
			IDs: m.deps.IDs, Clock: m.deps.Clock,
		}),
		Mute:           app.NewMuteHandler(m.deps.UoW, m.deps.Clock),
		Unmute:         app.NewUnmuteHandler(m.deps.UoW),
		PostChat:       app.NewPostChatMessageHandler(chat),
		DeleteChat:     app.NewDeleteChatMessageHandler(chat),
		MarkSeen:       app.NewMarkChatSeenHandler(chat),
		Token:          app.NewRealtimeTokenHandler(m.members, m.realtime),
		CreateComment:  app.NewCreateCommentHandler(comments),
		DeleteComment:  app.NewDeleteCommentHandler(comments),
		Removal:        app.NewRemoveCommentHandler(comments),
		LikeFeedItem:   app.NewLikeFeedItemHandler(likes),
		UnlikeFeedItem: app.NewUnlikeFeedItemHandler(likes),
		Match: app.NewMatchContactsHandler(app.MatchContactsDeps{
			UoW: m.deps.UoW, Users: m.users, Clock: m.deps.Clock,
		}),
		Members: m.members,
		Reads:   m.deps.Pool,
		Clock:   m.deps.Clock,
		Users:   m.users,
		Returns: m.returns,
		Logos:   m.logos, Pictures: m.pictures,
	}
}

func (m *Module) Consumers() []bus.Consumer {
	feed := adapters.Feed{
		Bus: m.deps.Bus, Users: m.users, Cabals: m.cabals, Assets: m.assets, Treasury: m.treasury,
		IDs: m.deps.IDs, UoW: m.deps.UoW,
	}
	referrals := adapters.ReferralFollows{Users: m.users, IDs: m.deps.IDs}
	revoke := adapters.RealtimeRevoke{Realtime: m.realtime}
	leftSettled := adapters.Settled[events.CabalMemberLeft]
	standingSettled := adapters.Settled[events.UserAccountStatusChanged]
	deletedSettled := adapters.Settled[events.UserDeleted]
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
				bus.Handle("social.feed.cabal_banned", feed.CabalBanned),
			},
		},
		{
			Durable:  "social_chat_seen_cleanup",
			Handlers: []bus.HandlerSpec{bus.Handle("social.chat_seen_cleanup", adapters.ChatSeenCleanup{}.Handle)},
		},
		{
			Durable: "social_realtime_revoke",
			Handlers: []bus.HandlerSpec{
				bus.HandleFetched("social.realtime_revoke.left", revoke.Left, leftSettled),
				bus.HandleFetched("social.realtime_revoke.standing", revoke.Standing, standingSettled),
				bus.HandleFetched("social.realtime_revoke.deleted", revoke.Deleted, deletedSettled),
			},
		},
		{
			Durable: "social_referral_follows",
			Handlers: []bus.HandlerSpec{
				bus.HandleFetched("social.referral_follows", referrals.Fetch, referrals.Apply),
			},
		},
	}
}

func (*Module) Pollers() []poller.Poller { return nil }
