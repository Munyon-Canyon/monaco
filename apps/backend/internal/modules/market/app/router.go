package app

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"math/big"
	"slices"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/concurrency"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	routerOp    = "market.Router.Best"
	shareNanos  = 1_000_000_000
	bpsWhole    = 10_000
	quoteFanOut = 8
)

type RouteListings interface {
	EnabledListings(ctx context.Context, stock multichain.StockID) ([]multichain.Listing, error)
}

type SharePrices interface {
	SharePrice(ctx context.Context, stock multichain.StockID) (money.Micros, error)
}

type BridgeQuotes interface {
	BridgeOut(ctx context.Context, to chain.Network, amount money.Micros) (money.Micros, error)
}

type RouterRules struct {
	MinEdgeBps   uint64
	MaxImpactBps uint64
	QuoteTimeout time.Duration
	MaxQuoteAge  time.Duration
}

type SimulationReadiness interface {
	Ready(ctx context.Context, n chain.Network) (bool, error)
}

type Venues map[multichain.Venue]multichain.Quoter

func (v Venues) Quote(
	ctx context.Context, l multichain.Listing, side multichain.Side, amount money.Micros,
) (multichain.Quote, error) {
	quoter, ok := v[l.Venue]
	if !ok {
		return multichain.Quote{}, errs.New(errs.CodeNoRoute, "market.Venues.Quote",
			slog.String("venue", string(l.Venue)))
	}
	return quoter.Quote(ctx, l, side, amount)
}

type Router struct {
	listings   RouteListings
	quoter     multichain.Quoter
	prices     SharePrices
	bridge     BridgeQuotes
	clock      clock.Clock
	rules      RouterRules
	simulation SimulationReadiness
}

func NewRouter(
	listings RouteListings, quoter multichain.Quoter, prices SharePrices,
	bridge BridgeQuotes, clk clock.Clock, rules RouterRules,
) *Router {
	return &Router{listings: listings, quoter: quoter, prices: prices, bridge: bridge, clock: clk, rules: rules}
}

func (r *Router) WithSimulationCheck(s SimulationReadiness) *Router {
	r.simulation = s
	return r
}

var (
	_ multichain.Router        = (*Router)(nil)
	_ multichain.NetworkRouter = (*Router)(nil)
)

type venueQuote struct {
	listing multichain.Listing
	quote   multichain.Quote
	ok      bool
	err     error
	reason  multichain.UnavailableReason
	detail  string
	took    time.Duration
}

type bridgeEstimate struct {
	once    sync.Once
	arrived money.Micros
	err     error
}

type bridgeEstimates map[chain.Network]*bridgeEstimate

type readiness struct {
	once  sync.Once
	ready bool
	err   error
}

type perChain struct {
	bridges bridgeEstimates
	ready   map[chain.Network]*readiness
}

func perChainFor(listings []multichain.Listing) perChain {
	out := perChain{bridges: bridgeEstimates{}, ready: map[chain.Network]*readiness{}}
	for _, l := range listings {
		if l.Network != chain.NetworkSolana {
			out.bridges[l.Network], out.ready[l.Network] = &bridgeEstimate{}, &readiness{}
		}
	}
	return out
}

func (r *Router) Best(
	ctx context.Context, stock multichain.StockID, side multichain.Side, amount money.Micros,
) (multichain.Comparison, error) {
	return r.best(ctx, stock, side, amount, "")
}

func (r *Router) BestOn(
	ctx context.Context, stock multichain.StockID, side multichain.Side, amount money.Micros, n chain.Network,
) (multichain.Comparison, error) {
	return r.best(ctx, stock, side, amount, n)
}

func (r *Router) best(
	ctx context.Context, stock multichain.StockID, side multichain.Side, amount money.Micros, on chain.Network,
) (multichain.Comparison, error) {
	if side != multichain.SideBuy || amount.IsZero() {
		return multichain.Comparison{}, errs.New(errs.CodeInvalidInput, routerOp,
			slog.String("side", string(side)), slog.String("amount_micros", amount.String()))
	}
	listings, err := r.listings.EnabledListings(ctx, stock)
	if err != nil {
		return multichain.Comparison{}, err
	}
	if on != "" {
		listings = slices.DeleteFunc(slices.Clone(listings), func(l multichain.Listing) bool { return l.Network != on })
	}
	ref, refErr := r.prices.SharePrice(ctx, stock)
	chains := perChainFor(listings)
	quotes, err := concurrency.FanOut(ctx, quoteFanOut, listings,
		func(ctx context.Context, l multichain.Listing) (venueQuote, error) {
			start := r.clock.Now()
			q := r.quote(ctx, l, amount, ref, refErr, chains)
			q.listing, q.took = l, r.clock.Now().Sub(start)
			return q, nil
		})
	if err != nil {
		return multichain.Comparison{}, errs.Wrap(err, errs.CodeInternal, routerOp)
	}
	return r.rank(ctx, stock, quotes)
}

func (r *Router) quote(
	ctx context.Context, l multichain.Listing, amount, ref money.Micros, refErr error, chains perChain,
) venueQuote {
	ctx, cancel := context.WithTimeout(ctx, r.rules.QuoteTimeout)
	defer cancel()
	if q, ready := r.simulationReady(ctx, l, chains.ready[l.Network]); !ready {
		return q
	}
	input, costs, err := r.bridgeIn(ctx, l, amount, refErr, chains.bridges)
	if err != nil && refErr != nil {
		return venueQuote{err: err, reason: multichain.UnavailableStale}
	}
	if err != nil {
		return failed(ctx, err)
	}
	q, err := r.quoter.Quote(ctx, l, multichain.SideBuy, input)
	if err != nil {
		return failed(ctx, err)
	}
	gross := grossShares(q.GrossOutput, l)
	fees := quotedCosts{
		poolFee: costOf(q.Costs, multichain.CostPoolFee),
		wrapper: costOf(q.Costs, multichain.CostPriceImpact),
	}
	impact, err := r.impact(l, gross, input, ref, refErr == nil, fees)
	if err != nil {
		return venueQuote{err: err, reason: multichain.UnavailableImpact}
	}
	gas := min(costOf(q.Costs, multichain.CostGas), input.Uint64())
	q.ListingID, q.Issuer, q.Network, q.Venue, q.Side = l.ID, l.Issuer, l.Network, l.Venue, multichain.SideBuy
	q.InputMicros, q.NetMicros = amount, amount
	q.Costs = append(append(costs, q.Costs...), impact...)
	q.NetShares = multichain.ShareNanos(scale(gross, input.Uint64()-gas, input.Uint64()))
	return venueQuote{quote: q, ok: true}
}

func (r *Router) simulationReady(ctx context.Context, l multichain.Listing, check *readiness) (venueQuote, bool) {
	if r.simulation == nil || check == nil {
		return venueQuote{}, true
	}
	check.once.Do(func() { check.ready, check.err = r.simulation.Ready(ctx, l.Network) })
	switch {
	case check.err != nil:
		return failed(ctx, check.err), false
	case !check.ready:
		return venueQuote{
			err: errs.New(errs.CodeNoRoute, routerOp, slog.String("listing_id", l.ID.String()),
				slog.String("network", string(l.Network)), slog.String("reason", notDelegated)),
			reason: multichain.UnavailableSimulationFailed, detail: NotDelegatedDetail,
		}, false
	}
	return venueQuote{}, true
}

const (
	notDelegated       = "simulation_account_not_delegated"
	NotDelegatedDetail = "not set up on this chain yet"
)

func failed(ctx context.Context, err error) venueQuote {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return venueQuote{
			err:    errs.Wrap(err, errs.CodeUpstreamUnavailable, routerOp),
			reason: multichain.UnavailableStale,
		}
	}
	if errs.CodeOf(err) == errs.CodeNoRoute {
		return venueQuote{err: err, reason: multichain.UnavailableNoRoute}
	}
	return venueQuote{err: err, reason: multichain.UnavailableSimulationFailed}
}

func (r *Router) bridgeIn(
	ctx context.Context, l multichain.Listing, amount money.Micros, refErr error, estimates bridgeEstimates,
) (money.Micros, []multichain.Cost, error) {
	if l.Network == chain.NetworkSolana {
		return amount, nil, nil
	}
	if refErr != nil {
		return money.Micros{}, nil, refErr
	}
	estimate := estimates[l.Network]
	estimate.once.Do(func() { estimate.arrived, estimate.err = r.bridge.BridgeOut(ctx, l.Network, amount) })
	arrived, err := estimate.arrived, estimate.err
	if err != nil {
		return money.Micros{}, nil, err
	}
	fee, err := amount.Sub(arrived)
	if err != nil || arrived.IsZero() {
		return money.Micros{}, nil, errs.New(errs.CodeUpstreamUnavailable, routerOp,
			slog.String("bridged_micros", arrived.String()))
	}
	return arrived, []multichain.Cost{{Kind: multichain.CostBridge, Micros: fee}}, nil
}

type quotedCosts struct{ poolFee, wrapper uint64 }

func (r *Router) impact(
	l multichain.Listing, gross uint64, input, ref money.Micros, priced bool, quoted quotedCosts,
) ([]multichain.Cost, error) {
	if !priced {
		return nil, nil
	}
	value := scale(gross, ref.Uint64(), shareNanos)
	if value >= input.Uint64() {
		return nil, nil
	}
	impact := input.Uint64() - value
	pool := impact - min(impact, quoted.wrapper)
	if pool*bpsWhole > r.rules.MaxImpactBps*input.Uint64() {
		return nil, errs.New(errs.CodeNoRoute, routerOp,
			slog.String("listing_id", l.ID.String()), slog.Uint64("impact_micros", pool))
	}
	if pool <= quoted.poolFee {
		return nil, nil
	}
	return []multichain.Cost{
		{Kind: multichain.CostPriceImpact, Micros: money.MicrosFromUint64(pool - quoted.poolFee)},
	}, nil
}

func grossShares(units *big.Int, l multichain.Listing) uint64 {
	if units == nil || units.Sign() <= 0 {
		return 0
	}
	n := new(big.Int).Mul(units, l.SharesPerToken.Num())
	n.Mul(n, big.NewInt(shareNanos))
	d := new(
		big.Int,
	).Mul(l.SharesPerToken.Denom(), new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(l.Decimals)), nil))
	n.Quo(n, d)
	if !n.IsUint64() {
		return 0
	}
	return n.Uint64()
}

func scale(v, num, den uint64) uint64 {
	n := new(big.Int).Mul(new(big.Int).SetUint64(v), new(big.Int).SetUint64(num))
	n.Quo(n, new(big.Int).SetUint64(den))
	if !n.IsUint64() {
		return 0
	}
	return n.Uint64()
}

func costOf(costs []multichain.Cost, kind multichain.CostKind) uint64 {
	var total uint64
	for _, c := range costs {
		if c.Kind == kind {
			total += c.Micros.Uint64()
		}
	}
	return total
}

func (r *Router) rank(
	ctx context.Context, stock multichain.StockID, quotes []venueQuote,
) (multichain.Comparison, error) {
	var ranked []multichain.Quote
	var unavailable []multichain.UnavailableVenue
	for _, q := range quotes {
		if logged, reason := r.dropReason(q); reason != "" {
			observability.Info(ctx, observability.MarketRouterVenueDropped,
				slog.String("listing_id", q.listing.ID.String()), slog.String("reason", logged),
				slog.String("err", errText(q.err)), slog.Int64("duration_ms", q.took.Milliseconds()))
			unavailable = append(unavailable, multichain.UnavailableVenue{
				ListingID: q.listing.ID, Network: q.listing.Network, Venue: q.listing.Venue, Reason: reason,
				Detail: q.detail,
			})
			continue
		}
		ranked = append(ranked, q.quote)
	}
	if len(ranked) == 0 {
		code := errs.CodeNoRoute
		if slices.ContainsFunc(unavailable, func(u multichain.UnavailableVenue) bool {
			return !unroutable(u.Reason) && u.Detail != NotDelegatedDetail
		}) {
			code = errs.CodeQuoteUnavailable
		}
		return multichain.Comparison{}, errs.New(code, routerOp, slog.String("stock_id", stock.String()))
	}
	slices.SortStableFunc(ranked, func(a, b multichain.Quote) int {
		return cmp.Or(cmp.Compare(b.NetShares, a.NetShares), cmp.Compare(solanaFirst(a), solanaFirst(b)))
	})
	winner, err := r.winner(ctx, stock, quotes, ranked)
	if err != nil {
		return multichain.Comparison{}, err
	}
	ranked = append([]multichain.Quote{ranked[winner]}, slices.Delete(slices.Clone(ranked), winner, winner+1)...)
	return multichain.Comparison{
		StockID:     stock,
		Side:        multichain.SideBuy,
		Ranked:      ranked,
		Winner:      ranked[0].ListingID,
		Unavailable: unavailable,
	}, nil
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func solanaFirst(q multichain.Quote) int {
	if q.Network == chain.NetworkSolana {
		return 0
	}
	return 1
}

func (r *Router) dropReason(q venueQuote) (string, multichain.UnavailableReason) {
	now := r.clock.Now()
	switch {
	case !q.ok && q.detail == NotDelegatedDetail:
		return notDelegated, q.reason
	case !q.ok:
		return string(errs.CodeOf(q.err)), q.reason
	case !now.Before(q.quote.ExpiresAt):
		return "expired", multichain.UnavailableExpired
	case now.Sub(q.quote.QuotedAt) > r.rules.MaxQuoteAge:
		return "stale", multichain.UnavailableStale
	case q.quote.NetShares == 0:
		return "no_output", multichain.UnavailableNoRoute
	}
	return "", ""
}

func (r *Router) winner(
	ctx context.Context, stock multichain.StockID, quotes []venueQuote, ranked []multichain.Quote,
) (int, error) {
	if ranked[0].Network == chain.NetworkSolana {
		return 0, nil
	}
	i := slices.IndexFunc(ranked, func(q multichain.Quote) bool { return q.Network == chain.NetworkSolana })
	if i < 0 {
		return 0, r.solanaUnquoted(ctx, stock, quotes)
	}
	evm, sol := uint64(ranked[0].NetShares), uint64(ranked[i].NetShares)
	if scale(evm, bpsWhole, 1) < scale(sol, bpsWhole+r.rules.MinEdgeBps, 1) {
		return i, nil
	}
	return 0, nil
}

func (r *Router) solanaUnquoted(ctx context.Context, stock multichain.StockID, quotes []venueQuote) error {
	i := slices.IndexFunc(quotes, func(q venueQuote) bool {
		_, reason := r.dropReason(q)
		return q.listing.Network == chain.NetworkSolana && !unroutable(reason)
	})
	if i < 0 {
		return nil
	}
	venue := string(quotes[i].listing.Venue)
	observability.Degraded(ctx, observability.MarketRouterSolanaUnquoted,
		slog.String("stock_id", stock.String()), slog.String("venue", venue))
	return errs.New(errs.CodeQuoteUnavailable, routerOp, slog.String("stock_id", stock.String()),
		slog.String("venue", venue))
}

func unroutable(reason multichain.UnavailableReason) bool {
	return reason == multichain.UnavailableNoRoute || reason == multichain.UnavailableImpact
}
