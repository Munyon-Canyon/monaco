package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	regularSession = 6*time.Hour + 30*time.Minute
	earlySession   = 3*time.Hour + 30*time.Minute
)

type CabalsHolding func(ctx context.Context, asset string) (int, error)

type Stats struct {
	MarketCap    string
	Open         *int64
	YearLow      *int64
	YearHigh     *int64
	HeldByCabals int
}

func (d *Detail) stats(ctx context.Context, item Summary, now time.Time) (Stats, error) {
	const op = "market.Detail.stats"
	asset := item.Asset
	since, until := sessionWindow(item.Session, now)
	var price int64
	if item.Priced {
		price, _ = domain.MicrosToInt64(item.Price.Micros)
	}
	row, err := d.facts.MarketStats(ctx, sqlc.MarketStatsParams{
		CompanyKey: asset.CompanyKey, Mint: asset.Mint.String(), PriceMicros: price, Since: since, Until: until,
	})
	if err != nil {
		return Stats{}, errs.Wrap(err, errs.CodeOf(err), op, slog.String("symbol", asset.Symbol))
	}
	out := Stats{MarketCap: row.MarketCapMicros}
	if row.OpenMicros > 0 {
		out.Open = &row.OpenMicros
	}
	if d.years != nil {
		low, high, yearErr := d.years.YearRange(ctx, asset.Mint, now)
		if yearErr != nil {
			return Stats{}, yearErr
		}
		out.YearLow, out.YearHigh = int64Ptr(low), int64Ptr(high)
	}
	if d.holding != nil {
		out.HeldByCabals, err = d.holding(ctx, string(asset.Mint.Address()))
		if err != nil {
			return Stats{}, errs.Wrap(err, errs.CodeOf(err), op, slog.String("symbol", asset.Symbol))
		}
	}
	return out, nil
}

func sessionWindow(session domain.SessionInfo, now time.Time) (since, until time.Time) {
	if session.State != domain.StateOpen {
		return session.LastSessionOpen, session.LastClose
	}
	length := regularSession
	if session.EarlyClose {
		length = earlySession
	}
	return session.NextTransition.Add(-length), now
}

func int64Ptr(m *money.Micros) *int64 {
	if m == nil {
		return nil
	}
	v, _ := domain.MicrosToInt64(*m)
	return &v
}
