package app

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const comparisonTTL = 15 * time.Second

type GetProposalComparison struct {
	CabalID    ids.CabalID
	ProposalID ids.ProposalID
	Caller     ids.UserID
}

type LiveComparison struct {
	QuoteOut   uint64
	Comparison *multichain.Comparison
	QuotedAt   time.Time
}

type ProposalComparisons struct {
	q        *sqlc.Queries
	propose  *ProposeTradeHandler
	listings PinListings
	mu       sync.Mutex
	cached   map[ids.ProposalID]*cachedComparison
}

type cachedComparison struct {
	born time.Time
	mu   sync.Mutex
	live LiveComparison
}

func NewProposalComparisons(db sqlc.DBTX, propose *ProposeTradeHandler, listings PinListings) *ProposalComparisons {
	return &ProposalComparisons{
		q: sqlc.New(db), propose: propose, listings: listings, cached: map[ids.ProposalID]*cachedComparison{},
	}
}

func (c *ProposalComparisons) Get(ctx context.Context, req GetProposalComparison) (LiveComparison, error) {
	const op = "governance.ProposalComparison"
	if err := member(ctx, c.propose.ports.Cabals, req.CabalID, req.Caller); err != nil {
		return LiveComparison{}, err
	}
	row, err := c.q.GetProposal(ctx, sqlc.GetProposalParams{ID: req.ProposalID.UUID(), CallerID: req.Caller.UUID()})
	if err != nil {
		return LiveComparison{}, lookupFailed(err, op)
	}
	if row.CabalID != req.CabalID.UUID() {
		return LiveComparison{}, errs.New(errs.CodeProposalNotFound, op)
	}
	if domain.Kind(row.Kind) != domain.KindBuy || domain.Status(row.Status) != domain.StatusOpen {
		return LiveComparison{}, errs.New(errs.CodeNotFound, op,
			slog.String("kind", row.Kind), slog.String("status", row.Status))
	}
	now := c.propose.clock.Now()
	entry := c.entry(req.ProposalID, now)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if fresh(entry.live, now) {
		return entry.live, nil
	}
	live, err := c.quote(ctx, row, now)
	if err != nil {
		return LiveComparison{}, err
	}
	entry.live = live
	return live, nil
}

func fresh(live LiveComparison, now time.Time) bool {
	return !live.QuotedAt.IsZero() && now.Sub(live.QuotedAt) < comparisonTTL
}

func (c *ProposalComparisons) entry(id ids.ProposalID, now time.Time) *cachedComparison {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.cached[id]; ok {
		return e
	}
	for key, e := range c.cached {
		if now.Sub(e.born) >= comparisonTTL && e.mu.TryLock() {
			if !fresh(e.live, now) {
				delete(c.cached, key)
			}
			e.mu.Unlock()
		}
	}
	e := &cachedComparison{born: now}
	c.cached[id] = e
	return e
}

func (c *ProposalComparisons) quote(
	ctx context.Context, row sqlc.GetProposalRow, now time.Time,
) (LiveComparison, error) {
	const op = "governance.ProposalComparison"
	asset, err := c.propose.ports.Assets.AssetBySymbol(ctx, row.Symbol)
	if err != nil {
		return LiveComparison{}, err
	}
	trade := domain.Trade{
		Kind: domain.KindBuy, Symbol: row.Symbol, USDCMicros: money.MicrosFromUint64(unsigned(row.UsdcMicros.Int64)),
	}
	routed, comparison, err := c.propose.route(ctx, asset, trade)
	if market.ProbeFailed(err) {
		err = errs.New(errs.CodeNoRoute, op)
	}
	if err != nil {
		return LiveComparison{}, err
	}
	network := chain.Network(row.Network)
	live := LiveComparison{Comparison: comparison, QuotedAt: now}
	priced, missed := pinned(comparison, network)
	if missed == nil {
		live.QuoteOut = displayQuote(asset, priced, routed, now)
	} else if live.Comparison, err = c.unquotedPin(ctx, row, asset, comparison); err != nil {
		return LiveComparison{}, err
	}
	observability.Debug(ctx, observability.GovernanceProposalComparisonRefreshed,
		slog.String("proposal_id", row.ID.String()), slog.Bool("compared", comparison != nil))
	return live, nil
}

func (c *ProposalComparisons) unquotedPin(
	ctx context.Context, row sqlc.GetProposalRow, asset market.Asset, live *multichain.Comparison,
) (*multichain.Comparison, error) {
	proposed, err := decodeComparison(row.PriceComparison)
	if err != nil {
		return nil, err
	}
	n := chain.Network(row.Network)
	unquoted := &multichain.Comparison{Side: multichain.SideBuy}
	venue, listed := multichain.UnavailableVenue{}, false
	for _, cmp := range []*multichain.Comparison{proposed, live} {
		if v, ok := pinVenue(cmp, n); ok {
			venue, unquoted.StockID, listed = v, cmp.StockID, true
		}
	}
	if !listed {
		if venue, err = c.listedPin(ctx, asset, n); err != nil {
			return nil, err
		}
		if venue.ListingID.IsZero() {
			observability.Info(ctx, observability.GovernanceProposalComparisonUnlisted,
				slog.String("proposal_id", row.ID.String()), slog.String("network", string(n)))
		}
	}
	unquoted.Winner, unquoted.Unavailable = venue.ListingID, []multichain.UnavailableVenue{venue}
	return unquoted, nil
}

func (c *ProposalComparisons) listedPin(
	ctx context.Context, asset market.Asset, n chain.Network,
) (multichain.UnavailableVenue, error) {
	venue := multichain.UnavailableVenue{
		Network: n,
		Venue:   multichain.VenueUniswap,
		Reason:  multichain.UnavailableNoRoute,
	}
	if !n.IsEVM() {
		venue.Venue = multichain.VenueJupiter
	}
	var stock multichain.StockID
	var id multichain.ListingID
	var on multichain.Venue
	found := false
	err := stock.UnmarshalText([]byte(asset.ID.String()))
	if err == nil {
		id, on, found, err = c.listings.PinListing(ctx, stock, n)
	}
	if found {
		venue.ListingID, venue.Venue = id, on
	}
	return venue, err
}

func pinVenue(c *multichain.Comparison, n chain.Network) (multichain.UnavailableVenue, bool) {
	if c == nil {
		return multichain.UnavailableVenue{}, false
	}
	for _, q := range c.Ranked {
		if q.Network == n {
			return multichain.UnavailableVenue{
				ListingID: q.ListingID, Network: n, Venue: q.Venue, Reason: multichain.UnavailableNoRoute,
			}, true
		}
	}
	for _, u := range c.Unavailable {
		if u.Network == n {
			return u, true
		}
	}
	return multichain.UnavailableVenue{}, false
}
