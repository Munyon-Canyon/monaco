package adapters

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/marketapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type HTTP struct {
	List   app.Lister
	Detail app.Detailer
	Chart  app.Charter
	Clock  clock.Clock
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
	body, err := wirePage(page)
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
	body, err := wireDetail(view, h.Clock.Now())
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
	chart, err := h.Chart.Handle(ctx, req.Symbol, string(req.Params.Range))
	if err != nil {
		return nil, err
	}
	body, err := wireChart(chart)
	if err != nil {
		return nil, err
	}
	return api.GetAssetChart200JSONResponse(body), nil
}

func listRequest(p api.GetAssetsParams) app.ListRequest {
	out := app.ListRequest{}
	if p.Q != nil {
		out.Q = *p.Q
	}
	if p.Filter != nil {
		out.Filter = app.Filter(*p.Filter)
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

func wirePage(page app.Page) (api.AssetList, error) {
	assets := make([]api.AssetSummary, len(page.Items))
	for i, item := range page.Items {
		wired, err := wireSummary(item)
		if err != nil {
			return api.AssetList{}, err
		}
		assets[i] = wired
	}
	return api.AssetList{Assets: assets, NextCursor: present(page.NextCursor)}, nil
}

func wireSummary(item app.Summary) (api.AssetSummary, error) {
	tradable := item.Asset.Tradable()
	out := api.AssetSummary{
		Symbol: item.Asset.Symbol, DisplayName: item.Asset.DisplayName,
		Issuer: api.AssetIssuer(item.Asset.Issuer), Kind: api.AssetKind(item.Asset.Kind),
		LogoUrl: present(item.Asset.LogoURL), Session: wireSession(item.Session), Tradable: &tradable,
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

func wireDetail(view app.AssetView, now time.Time) (api.AssetDetail, error) {
	summary, err := wireSummary(view.Summary)
	if err != nil {
		return api.AssetDetail{}, err
	}
	multiplier := view.Summary.Asset.UIMultiplierAt(now)
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
		Session:         summary.Session,
		Decimals:        int(view.Summary.Asset.Decimals),
		UiMultiplier:    api.UiMultiplier{Num: multiplier.Num, Den: multiplier.Den},
		Tradable:        view.Summary.Asset.Tradable(),
		OtherListings:   wireListings(view.Others),
		Attribution:     domain.Attribution,
	}, nil
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
	return api.AssetChart{
		Range: api.AssetChartRange(chart.Range), BucketSeconds: int64(chart.Bucket / time.Second),
		Points: points, Empty: chart.Empty, Attribution: domain.Attribution,
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
