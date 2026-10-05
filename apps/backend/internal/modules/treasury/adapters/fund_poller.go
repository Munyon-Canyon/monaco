package adapters

import (
	"context"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type FundPoller struct{ Settler *app.FundSettler }

func (FundPoller) Name() string { return "treasury.fund-transfers" }

func (FundPoller) Interval() time.Duration { return app.FundSettleInterval }

func (p FundPoller) Tick(ctx context.Context) (poller.Report, error) {
	tick, err := p.Settler.Tick(ctx)
	return poller.Report{
		Scanned: tick.Scanned, Changed: tick.Expired + tick.Changed + tick.Minted,
		Attrs: []slog.Attr{slog.Int("expired", tick.Expired), slog.Int("minted", tick.Minted)},
	}, err
}
