package adapters

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type WindDownPoller struct {
	WindDown *app.WindDown
	Every    time.Duration
}

func (WindDownPoller) Name() string { return "treasury.winddown" }

func (p WindDownPoller) Interval() time.Duration { return p.Every }

func (p WindDownPoller) Tick(ctx context.Context) (poller.Report, error) {
	scanned, changed, err := p.WindDown.Advance(ctx)
	return poller.Report{Scanned: scanned, Changed: changed}, err
}
