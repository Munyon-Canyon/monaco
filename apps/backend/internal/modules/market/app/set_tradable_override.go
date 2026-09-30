package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
)

func SetTradableOverride(
	ctx context.Context, q sqlc.DBTX, now time.Time, symbol string, o domain.Override,
) (domain.Asset, error) {
	row, err := sqlc.New(q).SetTradableOverride(ctx,
		sqlc.SetTradableOverrideParams{Override: string(o), Now: now, Symbol: symbol})
	return one(row, err, "market.SetTradableOverride", slog.String("symbol", symbol))
}
