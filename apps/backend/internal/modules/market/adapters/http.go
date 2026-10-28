package adapters

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	wire "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/marketapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type HTTP struct {
	List         app.Lister
	Detail       app.Detailer
	Chart        app.Charter
	BestChain    app.BestChainer
	Clock        clock.Clock
	PollInterval time.Duration
}

var _ api.StrictServerInterface = HTTP{}

func (h HTTP) GetAssets(
	ctx context.Context, req api.GetAssetsRequestObject,
) (api.GetAssetsResponseObject, error) {
	if err := caller(ctx); err != nil {
		return nil, err
	}
	page, err := h.List.Handle(ctx, listRequest(req.Params))
	if err != nil {
		return nil, err
	}
	body, err := wirePage(page, h.Clock.Now())
	if err != nil {
		return nil, err
	}
	return api.GetAssets200JSONResponse(body), nil
}

func (h HTTP) GetAsset(ctx context.Context, req api.GetAssetRequestObject) (api.GetAssetResponseObject, error) {
	if err := caller(ctx); err != nil {
		return nil, err
	}
	view, err := h.Detail.Handle(ctx, req.Symbol)
	if err != nil {
		return nil, err
	}
	body, err := wireDetail(view, h.Clock.Now(), h.PollInterval)
	if err != nil {
		return nil, err
	}
	return api.GetAsset200JSONResponse(body), nil
}

func (h HTTP) GetAssetChart(
	ctx context.Context, req api.GetAssetChartRequestObject,
) (api.GetAssetChartResponseObject, error) {
	if err := caller(ctx); err != nil {
		return nil, err
	}
	chart, err := h.Chart.Handle(ctx, req.Symbol, chartRequest(req.Params))
	if err != nil {
		return nil, err
	}
	body, err := wireChart(chart)
	if err != nil {
		return nil, err
	}
	return api.GetAssetChart200JSONResponse(body), nil
}

func (h HTTP) GetAssetBestChain(
	ctx context.Context, req api.GetAssetBestChainRequestObject,
) (api.GetAssetBestChainResponseObject, error) {
	if err := caller(ctx); err != nil {
		return nil, err
	}
	best, err := h.BestChain.Get(ctx, req.Symbol)
	if err != nil {
		return nil, err
	}
	return api.GetAssetBestChain200JSONResponse(wireBestChain(best)), nil
}

func chartRequest(p api.GetAssetChartParams) app.ChartRequest {
	out := app.ChartRequest{Range: string(p.Range), Before: p.Before}
	if p.Interval != nil {
		out.Interval = string(*p.Interval)
	}
	if p.Limit != nil {
		out.Limit = *p.Limit
	}
	return out
}

func listRequest(p api.GetAssetsParams) app.ListRequest {
	out := app.ListRequest{}
	if p.Q != nil {
		out.Q = *p.Q
	}
	if p.Filter != nil {
		out.Filter = app.Filter(*p.Filter)
	}
	if p.Sort != nil {
		out.Sort = app.Sort(*p.Sort)
	}
	if p.Limit != nil {
		out.Limit = *p.Limit
	}
	if p.Cursor != nil {
		out.Cursor = *p.Cursor
	}
	return out
}

func caller(ctx context.Context) error {
	const op = "market.caller"
	actor, ok := auth.ActorFrom(ctx)
	if !ok {
		return errs.New(errs.CodeUnauthorized, op)
	}
	if actor.Kind != auth.ActorUser {
		return errs.New(errs.CodeForbidden, op, slog.String("actor_kind", string(actor.Kind)))
	}
	if _, err := ids.ParseUserID(actor.ID); err != nil {
		return errs.Wrap(err, errs.CodeUnauthorized, op)
	}
	return nil
}

func wirePage(page app.Page, now time.Time) (api.AssetList, error) {
	assets := make([]api.AssetSummary, len(page.Items))
	for i, item := range page.Items {
		wired, err := wireSummary(item, now)
		if err != nil {
			return api.AssetList{}, err
		}
		assets[i] = wired
	}
	return api.AssetList{Assets: assets, NextCursor: present(page.NextCursor)}, nil
}

func wireSummary(item app.Summary, now time.Time) (api.AssetSummary, error) {
	tradable := item.Asset.Tradable()
	out := api.AssetSummary{
		Symbol: item.Asset.Symbol, DisplayName: item.Asset.DisplayName,
		Issuer: api.AssetIssuer(item.Asset.Issuer), Kind: api.AssetKind(item.Asset.Kind),
		LogoUrl: present(item.Asset.LogoURL), Session: wireSession(item.Session), Tradable: &tradable,
		Quotable: item.Asset.Quotable(now),
	}
	out.CabalCount = item.CabalCount
	if item.Profile != nil {
		estimate, err := wireEstimate(item.Profile.Estimate)
		if err != nil {
			return api.AssetSummary{}, err
		}
		out.Estimate = &estimate
	}
	if item.Volume != nil {
		volume, err := int64Micros(item.Volume.USD)
		if err != nil {
			return api.AssetSummary{}, err
		}
		asOf := item.Volume.At.UTC()
		out.Volume24hMicros = &volume
		out.Volume24hAsOf = &asOf
	}
	if !item.Priced {
		return out, nil
	}
	price, err := int64Micros(item.Price.Micros)
	if err != nil {
		return api.AssetSummary{}, err
	}
	at := item.Price.ObservedAt.UTC()
	points, err := int64Points(item.Sparkline)
	if err != nil {
		return api.AssetSummary{}, err
	}
	out.PriceMicros = &price
	out.PriceAsOf = &at
	out.ChangeBps = item.Change
	out.SparklineMicros = &points
	return out, nil
}

func wireDetail(view app.AssetView, now time.Time, pollInterval time.Duration) (api.AssetDetail, error) {
	summary, err := wireSummary(view.Summary, now)
	if err != nil {
		return api.AssetDetail{}, err
	}
	multiplier := view.Summary.Asset.UIMultiplierAt(now)
	var preIPO *api.PreIpoProfile
	if profile := view.Summary.Profile; profile != nil {
		wired, profileErr := wireProfile(*profile, *summary.Estimate)
		if profileErr != nil {
			return api.AssetDetail{}, profileErr
		}
		preIPO = &wired
	}
	var stats *api.AssetStats
	if view.Stats != nil {
		wired := wireStats(*view.Stats)
		stats = &wired
	}
	return api.AssetDetail{
		Symbol:          summary.Symbol,
		DisplayName:     summary.DisplayName,
		Issuer:          summary.Issuer,
		Kind:            summary.Kind,
		LogoUrl:         summary.LogoUrl,
		PriceMicros:     summary.PriceMicros,
		PriceAsOf:       summary.PriceAsOf,
		ChangeBps:       summary.ChangeBps,
		SparklineMicros: summary.SparklineMicros,
		Volume24hMicros: summary.Volume24hMicros,
		Volume24hAsOf:   summary.Volume24hAsOf,
		Session:         summary.Session,
		Quotable:        summary.Quotable,
		Decimals:        int(view.Summary.Asset.Decimals),
		UiMultiplier:    api.UiMultiplier{Num: multiplier.Num, Den: multiplier.Den},
		Tradable:        view.Summary.Asset.Tradable(),
		OtherListings:   wireListings(view.Others),
		Attribution:     domain.Attribution,
		PreIpo:          preIPO,
		Stats:           stats,

		PriceSource:              priceSource(view.Summary),
		PricePollIntervalSeconds: int(pollInterval / time.Second),
	}, nil
}

func wireStats(stats app.Stats) api.AssetStats {
	out := api.AssetStats{
		HeldByCabals: stats.HeldByCabals, OpenMicros: stats.Open,
		YearLowMicros: stats.YearLow, YearHighMicros: stats.YearHigh,
	}
	if stats.MarketCap != "" {
		out.MarketCapMicros = &stats.MarketCap
	}
	return out
}

func priceSource(item app.Summary) *api.AssetDetailPriceSource {
	if !item.Priced || item.Price.Source == "" {
		return nil
	}
	source := api.AssetDetailPriceSource(item.Price.Source)
	return &source
}

func wireChart(chart app.Chart) (api.AssetChart, error) {
	points := make([]api.ChartPoint, 0, len(chart.Points))
	for _, point := range chart.Points {
		wired, err := wirePoint(point)
		if err != nil {
			return api.AssetChart{}, err
		}
		points = append(points, wired)
	}
	var base *int64
	if chart.PreviousClose != nil {
		wired, err := int64Micros(*chart.PreviousClose)
		if err != nil {
			return api.AssetChart{}, err
		}
		base = &wired
	}
	var next *time.Time
	if chart.NextBefore != nil {
		utc := chart.NextBefore.UTC()
		next = &utc
	}
	return api.AssetChart{
		Range: api.AssetChartRange(chart.Range), Interval: api.AssetChartInterval(chart.Interval),
		BucketSeconds: int64(chart.Bucket / time.Second), Points: points, Empty: chart.Empty,
		Attribution: domain.Attribution, NextBefore: next, PreviousCloseMicros: base,
	}, nil
}

func wirePoint(point app.Point) (api.ChartPoint, error) {
	open, err := int64Micros(point.Open)
	high, highErr := int64Micros(point.High)
	low, lowErr := int64Micros(point.Low)
	last, lastErr := int64Micros(point.Close)
	if err = errors.Join(err, highErr, lowErr, lastErr); err != nil {
		return api.ChartPoint{}, err
	}
	return api.ChartPoint{
		T: point.At.UTC(), OpenMicros: open, HighMicros: high, LowMicros: low, CloseMicros: last,
	}, nil
}

func wireListings(items []app.Listing) []api.AssetListing {
	out := make([]api.AssetListing, len(items))
	for i, item := range items {
		out[i] = api.AssetListing{
			Symbol: item.Symbol, DisplayName: item.DisplayName,
			Issuer: api.AssetIssuer(item.Issuer), Kind: api.AssetKind(item.Kind),
			LogoUrl: present(item.LogoURL), Tradable: item.Tradable,
		}
	}
	return out
}

func wireSession(s domain.SessionInfo) api.MarketSession {
	out := api.MarketSession{
		State: api.MarketState(s.State), Continuous: s.Continuous, Holiday: s.Holiday, EarlyClose: s.EarlyClose,
	}
	if s.NextState != "" {
		next := api.MarketSessionNextState(s.NextState)
		out.NextState = &next
	}
	if !s.NextTransition.IsZero() {
		at := s.NextTransition.UTC()
		out.NextTransition = &at
	}
	return out
}

func int64Points(points []money.Micros) ([]int64, error) {
	out := make([]int64, len(points))
	for i, point := range points {
		n, err := int64Micros(point)
		if err != nil {
			return nil, err
		}
		out[i] = n
	}
	return out, nil
}

func int64Micros(m money.Micros) (int64, error) {
	n, ok := domain.MicrosToInt64(m)
	if !ok {
		return 0, errs.New(errs.CodeDecodeFailed, "market.int64Micros", slog.Uint64("micros", m.Uint64()))
	}
	return n, nil
}

func present(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

func wireBestChain(b app.BestChain) api.AssetBestChain {
	winner := b.Winner()
	return api.AssetBestChain{
		Symbol: b.Symbol, AmountMicros: b.Amount.String(), Network: wire.Network(winner.Network),
		Venue: api.AssetBestChainVenue(winner.Venue), ListingId: winner.ListingID.UUID(),
		NetShareNanos: shareNanos(winner.NetShares), QuotedAt: b.QuotedAt.UTC(),
		PriceComparison: wireComparison(b.Comparison), Stale: &b.Stale,
	}
}

func wireComparison(c multichain.Comparison) wire.PriceComparison {
	out := wire.PriceComparison{
		WinnerListingId: c.Winner.UUID(), Quotes: make([]wire.RouteQuote, len(c.Ranked)),
		Unavailable: make([]wire.UnavailableVenue, len(c.Unavailable)),
	}
	for i, u := range c.Unavailable {
		out.Unavailable[i] = wire.UnavailableVenue{
			ListingId: u.ListingID.UUID(), Network: wire.Network(u.Network),
			Venue: wire.UnavailableVenueVenue(u.Venue), Reason: wire.UnavailableVenueReason(u.Reason),
			ReasonDetail: present(u.Detail),
		}
	}
	for i, q := range c.Ranked {
		costs := make([]wire.RouteCost, len(q.Costs))
		for j, cost := range q.Costs {
			costs[j] = wire.RouteCost{Kind: wire.RouteCostKind(cost.Kind), Micros: cost.Micros.String()}
		}
		out.Quotes[i] = wire.RouteQuote{
			ListingId: q.ListingID.UUID(), Issuer: q.Issuer, Network: wire.Network(q.Network),
			Venue: wire.RouteQuoteVenue(q.Venue), NetShareNanos: shareNanos(q.NetShares),
			NetMicros: q.NetMicros.String(), Costs: costs, QuotedAt: q.QuotedAt, ExpiresAt: q.ExpiresAt,
		}
	}
	return out
}

func shareNanos(n multichain.ShareNanos) string { return strconv.FormatUint(uint64(n), 10) }

func wireEstimate(e domain.Estimate) (api.PreIpoEstimate, error) {
	price, err := int64Micros(e.Price)
	if err != nil {
		return api.PreIpoEstimate{}, err
	}
	return api.PreIpoEstimate{PriceMicros: price, AsOf: wireDate(e.AsOf)}, nil
}

func wireProfile(p domain.Profile, estimate api.PreIpoEstimate) (api.PreIpoProfile, error) {
	minOrder, err := int64Micros(p.MinOrder)
	if err != nil {
		return api.PreIpoProfile{}, err
	}
	rounds := make([]api.PreIpoRound, len(p.Rounds))
	for i, round := range p.Rounds {
		price, priceErr := int64Micros(round.Price)
		if priceErr != nil {
			return api.PreIpoProfile{}, priceErr
		}
		rounds[i] = api.PreIpoRound{On: wireDate(round.On), PriceMicros: price}
	}
	out := api.PreIpoProfile{
		Estimate: estimate, ValuationMicros: p.Valuation, Rounds: rounds, MinOrderMicros: minOrder,
		SellWindow: api.PreIpoSellWindow(p.SellWindow), ImageUrl: present(p.ImageURL),
	}
	if len(rounds) > 0 {
		out.LastRound = &rounds[len(rounds)-1]
	}
	if p.NextTenderOn != nil {
		next := wireDate(*p.NextTenderOn)
		out.NextTenderOn = &next
	}
	return out, nil
}

func wireDate(d domain.Date) openapi_types.Date { return openapi_types.Date{Time: d.Time()} }
