package app

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
)

const priceHintEvery = 30 * time.Second

const priceHintKey = "global.prices_updated"

type PriceHints struct {
	bus   *bus.Conn
	clock clock.Clock
	sent  metric.Int64Counter

	mu   sync.Mutex
	last time.Time
	once bool
}

func NewPriceHints(conn *bus.Conn, clk clock.Clock, meter metric.Meter) (*PriceHints, error) {
	counter, err := meter.Int64Counter("market_price_hints_total",
		metric.WithDescription("Global price hints published from price.tick, after coalescing."))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "market.NewPriceHints")
	}
	return &PriceHints{bus: conn, clock: clk, sent: counter}, nil
}

func (p *PriceHints) Subscribe(ctx context.Context) (func(), error) {
	return p.bus.SubscribeCore(ctx, "price.tick", func(ctx context.Context, _ []byte) {
		p.note(ctx)
	})
}

func (p *PriceHints) note(ctx context.Context) {
	now := p.clock.Now()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.once && now.Sub(p.last) < priceHintEvery {
		return
	}
	p.once = true
	p.last = now
	p.bus.PublishHint(ctx, priceHintKey, nil)
	p.sent.Add(ctx, 1)
}
