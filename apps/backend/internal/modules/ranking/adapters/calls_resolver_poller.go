package adapters

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const CallsResolverInterval = time.Minute

type CallsResolver interface {
	Run(ctx context.Context, now time.Time, limit int32) (app.ResolveReport, error)
}

type CallsResolverPoller struct {
	Resolver CallsResolver
	Clock    clock.Clock
}

func (CallsResolverPoller) Name() string { return "ranking.calls_resolver" }

func (CallsResolverPoller) Interval() time.Duration { return CallsResolverInterval }

func (p CallsResolverPoller) Tick(ctx context.Context) (poller.Report, error) {
	var total poller.Report
	for {
		got, err := p.Resolver.Run(ctx, p.Clock.Now(), app.CallsBatch)
		if err != nil {
			return total, err
		}
		total.Scanned += got.Scanned
		total.Changed += got.Resolved + got.Voided
		if got.Scanned < app.CallsBatch || got.Resolved+got.Voided == 0 {
			return total, nil
		}
	}
}
