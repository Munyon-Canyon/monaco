package social

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	marketport "github.com/monaco/monaco/apps/backend/internal/modules/market/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking"
	rankingport "github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters/ably"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Module struct {
	deps     module.Deps
	users    app.Users
	returns  app.Returns
	tallies  app.Tallies
	members  app.ChatMembers
	cabals   app.Cabals
	realtime app.Realtime
	assets   app.Assets
	logos    app.AssetLogos
	pictures app.CabalPictures
	treasury app.Treasury
	holdings app.Holdings
	activity app.MemberActivity
	tops     app.ClosedTops
	catalog  market.Catalog
	prices   market.Prices
	stocks   app.StockCards
}

type Option func(*Module)

func WithUsers(users app.Users) Option {
	return func(m *Module) { m.users = users }
}

func WithReturns(returns app.Returns) Option {
	return func(m *Module) { m.returns = returns }
}

func WithTallies(tallies app.Tallies) Option {
	return func(m *Module) { m.tallies = tallies }
}

func WithCabals(cabals app.Cabals) Option {
	return func(m *Module) { m.cabals = cabals }
}

func WithCabalPictures(pictures app.CabalPictures) Option {
	return func(m *Module) { m.pictures = pictures }
}

func WithHoldings(holdings app.Holdings) Option {
	return func(m *Module) { m.holdings = holdings }
}

func WithTreasury(treasury app.Treasury) Option {
	return func(m *Module) { m.treasury = treasury }
}

func WithAssets(catalog market.Catalog) Option {
	return func(m *Module) {
		m.catalog = catalog
		assets := NewAssets(catalog).WithPrices(m.prices, m.deps.Clock)
		m.assets, m.logos = assets, assets
	}
}

func WithPrices(prices market.Prices) Option {
	return func(m *Module) { m.prices = prices }
}

type CatalogAssets struct {
	catalog market.Catalog
	prices  market.Prices
	clock   clock.Clock
}

func NewAssets(catalog market.Catalog) CatalogAssets { return CatalogAssets{catalog: catalog} }

func (c CatalogAssets) WithPrices(prices market.Prices, clk clock.Clock) CatalogAssets {
	c.prices, c.clock = prices, clk
	return c
}

const stockChangeWindow = 24 * time.Hour

func (c CatalogAssets) StockCards(ctx context.Context, assetIDs []uuid.UUID) (map[uuid.UUID]app.StockCard, error) {
	cards := make(map[uuid.UUID]app.StockCard, len(assetIDs))
	var known []market.AssetID
	for _, id := range assetIDs {
		asset, err := c.catalog.AssetByID(ctx, market.AssetIDFrom(id))
		switch {
		case errs.CodeOf(err) == errs.CodeAssetNotFound:
			continue
		case err != nil:
			return nil, err
		}
		cards[id] = app.StockCard{
			ID: id, Symbol: asset.Symbol, Name: asset.DisplayName, LogoURL: asset.LogoURL,
			IsPreIPO: string(asset.Kind) == string(feed.AssetKindPreIPO),
		}
		known = append(known, asset.ID)
	}
	if len(known) == 0 {
		return cards, nil
	}
	latest, err := c.prices.LatestPrices(ctx)
	if err != nil {
		return nil, err
	}
	earlier, err := c.prices.PricesAsOf(ctx, known, c.clock.Now().Add(-stockChangeWindow))
	if err != nil {
		return nil, err
	}
	for _, asset := range known {
		card := cards[asset.UUID()]
		now := latest[asset]
		card.PriceMicros = now.Micros
		if then, ok := earlier[asset]; ok && !card.IsPreIPO {
			if bps, measured := market.QuoteBps(now.Micros, then.Micros); measured {
				card.ChangeBps = &bps
			}
		}
		cards[asset.UUID()] = card
	}
	return cards, nil
}

func (c CatalogAssets) AssetByMint(ctx context.Context, raw string) (app.AssetCard, error) {
	mint, err := market.ParseMint(raw)
	if err != nil {
		return app.AssetCard{}, err
	}
	asset, err := c.catalog.AssetByMint(ctx, mint)
	return app.AssetCard{
		ID: asset.ID.UUID(), Symbol: asset.Symbol, Name: asset.DisplayName, Decimals: asset.Decimals,
		Kind: string(asset.Kind),
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
	m := &Module{deps: d, activity: app.UnwiredMemberActivity{}, tops: app.UnwiredClosedTops{}}
	for _, opt := range opts {
		opt(m)
	}
	if m.users == nil {
		m.users = identity.New(d).Queries()
	}
	if m.returns == nil {
		m.returns = ranking.New(d).Queries()
	}
	if m.catalog == nil {
		m.catalog = market.New(d).Catalog()
	}
	if m.prices == nil {
		m.prices = market.New(d).Prices()
	}
	stocks := NewAssets(m.catalog).WithPrices(m.prices, m.deps.Clock)
	m.stocks = stocks
	if m.assets == nil {
		m.assets, m.logos = stocks, stocks
	}
	if m.treasury == nil {
		m.treasury = treasury.New(d).Queries()
	}
	if m.holdings == nil {
		m.holdings = PositionHoldings{Positions: treasury.New(d).Queries(), Swaps: treasury.New(d).Swaps()}
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

func (m *Module) Wire(set module.Set) {
	for _, mod := range set {
		if provider, ok := mod.(interface{ Tallies() governanceport.Tallies }); ok {
			m.tallies = provider.Tallies()
		}
		if provider, ok := mod.(interface {
			MemberActivity() governanceport.MemberActivity
		}); ok {
			m.activity = provider.MemberActivity()
		}
		if provider, ok := mod.(interface{ CallBoards() rankingport.CallBoards }); ok {
			m.tops = app.MonthTops{Boards: provider.CallBoards()}
		}
	}
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

type TopReply = app.TopReply

func (m *Module) ReplyCounts(ctx context.Context, proposalIDs []ids.ProposalID) (map[ids.ProposalID]int, error) {
	return app.ReplyCounts(ctx, m.deps.Pool, proposalIDs)
}

func (m *Module) TopReply(
	ctx context.Context, proposalIDs []ids.ProposalID, start, end time.Time,
) (*TopReply, error) {
	top, found, err := app.FindTopReply(ctx, m.deps.Pool, proposalIDs, start, end)
	if err != nil || !found {
		return nil, err
	}
	return &top, nil
}

func (m *Module) CommentLiveness() app.CommentLiveness { return app.NewCommentLiveness(m.deps.Pool) }

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

func (m *Module) Blocks() interface {
	BlockedEitherWay(context.Context, ids.UserID, ids.UserID) (bool, error)
} {
	return app.NewActivityStore(m.deps.Pool)
}

func (m *Module) CommentPreviews() app.CommentPreviews {
	return app.NewCommentPreviews(m.deps.Pool, m.users)
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
		Clock: m.deps.Clock, Hints: m.deps.Bus, Stocks: m.stocks,
	}
	likes := app.LikeDeps{
		UoW: m.deps.UoW, Reads: m.deps.Pool, Members: m.members, Clock: m.deps.Clock, Hints: m.deps.Bus,
	}
	polls := app.PollDeps{
		UoW: m.deps.UoW, Reads: m.deps.Pool, Members: m.members, Clock: m.deps.Clock, Hints: m.deps.Bus,
	}
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
		Mute:                    app.NewMuteHandler(m.deps.UoW, m.deps.Clock),
		Unmute:                  app.NewUnmuteHandler(m.deps.UoW),
		PostChat:                app.NewPostChatMessageHandler(chat),
		DeleteChat:              app.NewDeleteChatMessageHandler(chat),
		MarkSeen:                app.NewMarkChatSeenHandler(chat),
		Token:                   app.NewRealtimeTokenHandler(m.members, m.realtime),
		CreateComment:           app.NewCreateCommentHandler(comments),
		DeleteComment:           app.NewDeleteCommentHandler(comments),
		Removal:                 app.NewRemoveCommentHandler(comments),
		VoteCommentPoll:         app.NewVoteCommentPollHandler(polls),
		WithdrawCommentPollVote: app.NewWithdrawCommentPollVoteHandler(polls),
		LikeFeedItem:            app.NewLikeFeedItemHandler(likes),
		UnlikeFeedItem:          app.NewUnlikeFeedItemHandler(likes),
		ReactFeedItem:           app.NewReactFeedItemHandler(likes),
		UnreactFeedItem:         app.NewUnreactFeedItemHandler(likes),
		ReactComment:            app.NewReactCommentHandler(likes),
		UnreactComment:          app.NewUnreactCommentHandler(likes),
		Match: app.NewMatchContactsHandler(app.MatchContactsDeps{
			UoW: m.deps.UoW, Users: m.users, Clock: m.deps.Clock,
		}),
		Members: m.members,
		Reads:   m.deps.Pool,
		Stocks:  m.stocks,
		Clock:   m.deps.Clock,
		Users:   m.users,
		Returns: m.returns,
		Tallies: m.tallies,
		Logos:   m.logos, Pictures: m.pictures,
		History: chartPrices{market.New(m.deps).History()},
		Marks:   chartTrades{treasury.New(m.deps).TradeMarks()},
		Activity: &app.UserActivityQuery{
			Store: app.NewActivityStore(m.deps.Pool), Users: m.users, Cabals: m.pictures,
			Activity: m.activity, Tops: m.tops,
		},
	}
}

func (m *Module) Consumers() []bus.Consumer {
	feed := adapters.Feed{
		Bus: m.deps.Bus, Users: m.users, Cabals: m.cabals, Assets: m.assets, Treasury: m.treasury,
		Tallies: m.tallies, IDs: m.deps.IDs, UoW: m.deps.UoW,
	}
	thread := adapters.Thread{
		Hints: m.deps.Bus, Assets: m.assets, Holdings: m.holdings, Tallies: m.tallies,
	}
	referrals := adapters.ReferralFollows{Users: m.users, IDs: m.deps.IDs}
	revoke := adapters.RealtimeRevoke{Realtime: m.realtime}
	leftSettled := adapters.Settled[events.CabalMemberLeft]
	standingSettled := adapters.Settled[events.UserAccountStatusChanged]
	deletedSettled := adapters.Settled[events.UserDeleted]
	return append(cleanupConsumers(), []bus.Consumer{
		{
			Durable: "social_feed",
			Handlers: []bus.HandlerSpec{
				bus.HandleFetched("social.feed", feed.FetchCreated, feed.ApplyCreated),
				bus.HandleFetched("social.feed.joined", feed.FetchJoined, feed.ApplyJoined),
				bus.Handle("social.feed.left", feed.Left),
				bus.HandleFetched("social.feed.profile_updated", feed.FetchProfile, feed.ApplyProfile),
				bus.HandleFetched("social.feed.cabal_updated", feed.FetchCabal, feed.ApplyCabal),
				bus.HandleFetched("social.feed.proposal_created", feed.FetchProposal, feed.ApplyProposal),
				bus.HandleFetched("social.feed.proposal_passed", feed.FetchPassed, feed.ApplyPassed),
				bus.HandleFetched("social.feed.proposal_failed", feed.FetchFailed, feed.ApplyFailed),
				bus.HandleFetched("social.feed.proposal_expired", feed.FetchExpired, feed.ApplyExpired),
				bus.Handle("social.feed.vote_cast", feed.VoteCast),
				bus.Handle("social.feed.vote_retracted", feed.VoteRetracted),
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
			Durable: "social_thread",
			Handlers: []bus.HandlerSpec{
				bus.HandleFetched("social.thread.trade_confirmed", thread.FetchConfirmed, thread.ApplyConfirmed),
				bus.HandleFetched("social.thread.proposal_failed", thread.FetchClosed, thread.ApplyFailed),
				bus.HandleFetched("social.thread.proposal_expired", thread.FetchExpired, thread.ApplyExpired),
				bus.Handle("social.thread.trade_failed", thread.TradeFailed),
			},
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
	}...)
}

func cleanupConsumers() []bus.Consumer {
	return []bus.Consumer{
		{
			Durable: "social_chat_seen_cleanup",
			Handlers: []bus.HandlerSpec{
				bus.Handle("social.chat_seen_cleanup", adapters.ChatSeenCleanup{}.Handle),
				bus.Handle("social.chat_seen_seed", adapters.ChatSeenCleanup{}.Joined),
			},
		},
		{
			Durable:  "social_deleted_cleanup",
			Handlers: []bus.HandlerSpec{bus.Handle("social.deleted_cleanup", adapters.DeletedCleanup{}.Handle)},
		},
	}
}

func (*Module) Pollers() []poller.Poller { return nil }

type chartPrices struct{ history marketport.History }

func (c chartPrices) Latest(ctx context.Context, assetIDs []uuid.UUID) (map[uuid.UUID]money.Micros, error) {
	return c.history.Latest(ctx, assetIDs)
}

func (c chartPrices) Closes(
	ctx context.Context, windows []app.ChartWindow, buckets int,
) (map[uuid.UUID][]app.ChartPoint, error) {
	in := make([]marketport.Window, len(windows))
	for i, w := range windows {
		in[i] = marketport.Window{Key: w.Key, AssetID: w.AssetID, From: w.From, To: w.To}
	}
	closes, err := c.history.Closes(ctx, in, buckets)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID][]app.ChartPoint, len(closes))
	for key, points := range closes {
		out[key] = make([]app.ChartPoint, len(points))
		for i, p := range points {
			out[key][i] = app.ChartPoint{At: p.At, PriceMicros: p.PriceMicros}
		}
	}
	return out, nil
}

type chartTrades struct{ marks treasuryport.TradeMarks }

func (c chartTrades) TradeMarks(
	ctx context.Context, pairs []app.ChartPair,
) (map[app.ChartPair][]app.ChartTrade, error) {
	in := make([]treasuryport.CabalAsset, len(pairs))
	for i, p := range pairs {
		in[i] = treasuryport.CabalAsset{CabalID: p.CabalID, Symbol: p.Symbol}
	}
	marks, err := c.marks.TradeMarks(ctx, in)
	if err != nil {
		return nil, err
	}
	out := make(map[app.ChartPair][]app.ChartTrade, len(marks))
	for pair, list := range marks {
		trades := make([]app.ChartTrade, len(list))
		for i, t := range list {
			trades[i] = app.ChartTrade{
				At: t.At, Side: t.Side, USDCMicros: t.USDCMicros, PriceMicros: t.PriceMicros,
				CostOfSoldMicros: t.CostOfSoldMicros, CycleStart: t.CycleStart,
			}
		}
		out[app.ChartPair{CabalID: pair.CabalID, Symbol: pair.Symbol}] = trades
	}
	return out, nil
}

type PositionReader interface {
	SolanaPositions(ctx context.Context, cabalID ids.CabalID) ([]treasuryport.Position, error)
}

type SwapReader interface {
	SwapPosted(ctx context.Context, swapID uuid.UUID) (bool, error)
}

type PositionHoldings struct {
	Positions PositionReader
	Swaps     SwapReader
}

func (p PositionHoldings) Posted(ctx context.Context, swapID uuid.UUID) (bool, error) {
	return p.Swaps.SwapPosted(ctx, swapID)
}

func (p PositionHoldings) Holding(ctx context.Context, cabalID ids.CabalID, mint chain.SolanaAddress) (uint64, error) {
	positions, err := p.Positions.SolanaPositions(ctx, cabalID)
	if err != nil {
		return 0, err
	}
	for _, position := range positions {
		if position.Mint == mint {
			return position.Units.Uint64(), nil
		}
	}
	return 0, nil
}
