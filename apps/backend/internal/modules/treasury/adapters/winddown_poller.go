package adapters

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type WindDownPoller struct {
	WindDown *app.WindDown
}

func (WindDownPoller) Name() string { return "treasury.winddown" }

func (WindDownPoller) Interval() time.Duration { return app.WindDownInterval }

func (p WindDownPoller) Tick(ctx context.Context) (poller.Report, error) {
	scanned, changed, err := p.WindDown.Advance(ctx)
	return poller.Report{Scanned: scanned, Changed: changed}, err
}
