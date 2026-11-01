package app

import (
	"context"
	"log/slog"
	"math/big"
	"sort"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type HoldingPerformanceQuery struct {
	CabalID ids.CabalID
	Caller  ids.UserID
	Symbol  string
}

const maxSeriesPoints = 48

func SeriesRanges() []string { return []string{"1W", "1M", "3M", "1Y", "ALL"} }

type PricePoint struct {
	At          time.Time
	PriceMicros money.Micros
}

type PositionPrices func(ctx context.Context, symbol, chartRange string) ([]PricePoint, error)

type PnLPoint struct {
	At        time.Time
	PnLMicros money.SignedMicros
}

type HoldingPerformance struct {
	AvgSellMicros     *money.Micros
	RealizedPnLMicros money.SignedMicros
	Series            map[string][]PnLPoint
}

type HoldingPerformanceReads struct {
	q        *sqlc.Queries
	members  Members
	assets   AssetNames
	listings ListingReader
	prices   PositionPrices
	clock    clock.Clock
}

func NewHoldingPerformanceReads(
	db sqlc.DBTX, members Members, assets AssetNames, listings ListingReader, prices PositionPrices, c clock.Clock,
) *HoldingPerformanceReads {
	return &HoldingPerformanceReads{
		q: sqlc.New(db), members: members, assets: assets, listings: listings, prices: prices, clock: c,
	}
}

func (r *HoldingPerformanceReads) Handle(
	ctx context.Context, req HoldingPerformanceQuery,
) (HoldingPerformance, error) {
	const op = "treasury.HoldingPerformance"
	member, err := r.members.IsMember(ctx, req.CabalID, req.Caller)
	if err != nil {
		return HoldingPerformance{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	if !member {
		return HoldingPerformance{}, errs.New(
			errs.CodeNotCabalMember,
			op,
			slog.String("cabal_id", req.CabalID.String()),
		)
	}
	names, err := r.assets.AssetNames(ctx)
	if err != nil {
		return HoldingPerformance{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	mint, ok := mintOf(names, req.Symbol)
	if !ok {
		return HoldingPerformance{}, nil
	}
	keys, quotes, err := stockListings(ctx, r.q, r.listings, req.CabalID, mint)
	if err != nil {
		return HoldingPerformance{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	params := sqlc.ListTradesForPairsParams{}
	for _, key := range keys {
		params.CabalIds = append(params.CabalIds, req.CabalID.UUID())
		params.Assets = append(params.Assets, key)
	}
	rows, err := r.q.ListTradesForPairs(ctx, params)
	if err != nil {
		return HoldingPerformance{}, errs.Wrap(err, errs.CodeDBUnavailable, op)
	}
	sales, line, err := replay(rows, names, quotes)
	if err != nil {
		return HoldingPerformance{}, err
	}
	series, err := r.series(ctx, req.Symbol, line)
	if err != nil {
		return HoldingPerformance{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	out, err := sales.result()
	out.Series = series
	return out, err
}

func replay(
	rows []sqlc.ListTradesForPairsRow, names map[domain.Asset]AssetName, quotes map[string]ListingQuote,
) (*salesTotal, *pnlTimeline, error) {
	sales := &salesTotal{}
	positions := map[string]*replayed{}
	line := &pnlTimeline{held: map[string]*big.Rat{}, perToken: perTokenFor(names, quotes)}
	for _, row := range rows {
		tr, err := tradeOf(row)
		if err != nil {
			return nil, nil, err
		}
		state, ok := positions[row.Asset]
		if !ok {
			state = &replayed{}
			positions[row.Asset] = state
		}
		shares := sharesFor(row.Asset, names, quotes)
		if err := sales.apply(state, tr, shares); err != nil {
			return nil, nil, err
		}
		if err := line.record(tr, row.Asset, positions, sales); err != nil {
			return nil, nil, err
		}
	}
	return sales, line, nil
}

type pnlStep struct {
	at   time.Time
	held map[string]*big.Rat
	net  *big.Int
}

type perTokenAt func(asset string, at time.Time) (*big.Rat, uint8, error)

type pnlTimeline struct {
	steps    []pnlStep
	held     map[string]*big.Rat
	perToken perTokenAt
}

func (l *pnlTimeline) record(tr trade, asset string, positions map[string]*replayed, sales *salesTotal) error {
	_, decimals, err := l.perToken(asset, tr.at)
	if err != nil {
		return err
	}
	whole := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	moved := new(big.Rat).SetFrac(new(big.Int).SetUint64(tr.units), whole)
	tokens := new(big.Rat)
	if prior := l.held[asset]; prior != nil {
		tokens.Set(prior)
	}
	if tr.side == sideBuy {
		tokens.Add(tokens, moved)
	} else {
		tokens.Sub(tokens, moved)
	}
	l.held[asset] = tokens
	net := new(big.Int)
	for _, state := range positions {
		net.Add(net, new(big.Int).SetUint64(state.cost))
	}
	realized := new(big.Int).SetUint64(sales.proceeds.Uint64())
	realized.Sub(realized, new(big.Int).SetUint64(sales.removed.Uint64()))
	held := make(map[string]*big.Rat, len(l.held))
	for key, value := range l.held {
		held[key] = new(big.Rat).Set(value)
	}
	l.steps = append(l.steps, pnlStep{at: tr.at, held: held, net: net.Sub(net, realized)})
	return nil
}

func (l *pnlTimeline) pnlAt(at time.Time, price money.Micros) (money.SignedMicros, error) {
	i := sort.Search(len(l.steps), func(i int) bool { return l.steps[i].at.After(at) })
	if i == 0 {
		return money.SignedMicrosFromInt64(0), nil
	}
	step := l.steps[i-1]
	shares := new(big.Rat)
	for asset, tokens := range step.held {
		per, _, err := l.perToken(asset, at)
		if err != nil {
			return money.SignedMicros{}, err
		}
		shares.Add(shares, new(big.Rat).Mul(tokens, per))
	}
	value := new(big.Rat).Mul(shares, new(big.Rat).SetInt(new(big.Int).SetUint64(price.Uint64())))
	value.Sub(value, new(big.Rat).SetInt(step.net))
	return money.SignedMicrosFromInt64(new(big.Int).Quo(value.Num(), value.Denom()).Int64()), nil
}

func (r *HoldingPerformanceReads) series(
	ctx context.Context, symbol string, line *pnlTimeline,
) (map[string][]PnLPoint, error) {
	out := make(map[string][]PnLPoint, len(SeriesRanges()))
	if len(line.steps) == 0 || r.prices == nil {
		return out, nil
	}
	now := r.clock.Now().UTC()
	first := line.steps[0].at
	for _, name := range SeriesRanges() {
		prices, err := r.prices(ctx, symbol, name)
		if err != nil {
			return nil, err
		}
		if len(prices) == 0 {
			continue
		}
		points, err := seriesPoints(line, prices, first, now)
		if err != nil {
			return nil, err
		}
		out[name] = thinSeries(points)
	}
	return out, nil
}

func seriesPoints(
	line *pnlTimeline, prices []PricePoint, first, now time.Time,
) ([]PnLPoint, error) {
	points := make([]PnLPoint, 0, len(prices)+1)
	for _, p := range prices {
		if p.At.Before(first) || !p.At.Before(now) {
			continue
		}
		pnl, err := line.pnlAt(p.At, p.PriceMicros)
		if err != nil {
			return nil, err
		}
		points = append(points, PnLPoint{At: p.At, PnLMicros: pnl})
	}
	last, err := line.pnlAt(now, prices[len(prices)-1].PriceMicros)
	if err != nil {
		return nil, err
	}
	return append(points, PnLPoint{At: now, PnLMicros: last}), nil
}

func thinSeries(points []PnLPoint) []PnLPoint {
	if len(points) <= maxSeriesPoints {
		return points
	}
	out := make([]PnLPoint, maxSeriesPoints)
	for i := range out {
		out[i] = points[i*(len(points)-1)/(maxSeriesPoints-1)]
	}
	return out
}

type sharesOf func(tr trade) (*big.Rat, error)

func sharesFor(
	asset string, names map[domain.Asset]AssetName, quotes map[string]ListingQuote,
) sharesOf {
	return func(tr trade) (*big.Rat, error) {
		per, decimals, err := sharesPerToken(asset, tr, names, quotes)
		if err != nil {
			return nil, err
		}
		whole := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
		units := new(big.Int).SetUint64(tr.units)
		units.Mul(units, per.Num())
		return new(big.Rat).SetFrac(units, new(big.Int).Mul(per.Denom(), whole)), nil
	}
}

func sharesPerToken(
	asset string, tr trade, names map[domain.Asset]AssetName, quotes map[string]ListingQuote,
) (*big.Rat, uint8, error) {
	return perTokenFor(names, quotes)(asset, tr.at)
}

func perTokenFor(names map[domain.Asset]AssetName, quotes map[string]ListingQuote) perTokenAt {
	return func(asset string, at time.Time) (*big.Rat, uint8, error) {
		return perTokenAtTime(asset, at, names, quotes)
	}
}

func perTokenAtTime(
	asset string, at time.Time, names map[domain.Asset]AssetName, quotes map[string]ListingQuote,
) (*big.Rat, uint8, error) {
	const op = "treasury.HoldingPerformance.shares"
	if quote, listed := quotes[asset]; listed {
		if quote.SharesPerToken == nil || quote.SharesPerToken.Sign() <= 0 {
			return nil, 0, errs.New(errs.CodeInternal, op, slog.String("asset", asset))
		}
		return quote.SharesPerToken, quote.Decimals, nil
	}
	name, known := names[domain.Asset(asset)]
	if !known {
		return nil, 0, errs.New(errs.CodeInternal, op, slog.String("asset", asset))
	}
	if name.MultiplierAt == nil {
		return big.NewRat(1, 1), name.Decimals, nil
	}
	num, den := name.MultiplierAt(at)
	if num <= 0 || den <= 0 {
		return nil, 0, errs.New(errs.CodeInternal, op, slog.String("asset", asset))
	}
	return big.NewRat(num, den), name.Decimals, nil
}

type salesTotal struct {
	proceeds money.Micros
	removed  money.Micros
	shares   big.Rat
}

func (s *salesTotal) apply(state *replayed, tr trade, shares sharesOf) error {
	const op = "treasury.HoldingPerformance.apply"
	switch tr.side {
	case sideBuy:
		return state.buy(tr)
	case sideSell:
		removed, err := state.sell(tr)
		if err != nil {
			return err
		}
		sold, err := shares(tr)
		if err != nil {
			return err
		}
		if s.removed, err = s.removed.Add(removed); err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		if s.proceeds, err = s.proceeds.Add(money.MicrosFromUint64(tr.usdc)); err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		s.shares.Add(&s.shares, sold)
		return nil
	}
	return errs.New(errs.CodeInternal, op, slog.String("side", tr.side))
}

func (s *salesTotal) result() (HoldingPerformance, error) {
	const op = "treasury.HoldingPerformance.result"
	realized, err := s.proceeds.Delta(s.removed)
	if err != nil {
		return HoldingPerformance{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	out := HoldingPerformance{RealizedPnLMicros: realized}
	if s.shares.Sign() == 0 {
		return out, nil
	}
	avg := new(big.Int).SetUint64(s.proceeds.Uint64())
	avg.Mul(avg, s.shares.Denom())
	avg.Quo(avg, s.shares.Num())
	if !avg.IsUint64() {
		return HoldingPerformance{}, errs.New(errs.CodeInternal, op, slog.String("avg", avg.String()))
	}
	micros := money.MicrosFromUint64(avg.Uint64())
	out.AvgSellMicros = &micros
	return out, nil
}
