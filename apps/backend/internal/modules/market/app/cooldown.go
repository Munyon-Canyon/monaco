package app

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const minRateLimitCooldown = 60 * time.Second

type Cooldown struct {
	clock clock.Clock
	mu    sync.Mutex
	until time.Time
}

func NewCooldown(c clock.Clock) *Cooldown { return &Cooldown{clock: c} }

func (c *Cooldown) Active() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.clock.Now().Before(c.until)
}

func (c *Cooldown) Trip(ctx context.Context, poller string, err error) bool {
	if errs.CodeOf(err) != errs.CodeCoinGeckoRateLimited {
		return false
	}
	after := minRateLimitCooldown
	for _, a := range errs.Detail(err) {
		if a.Key == "retry_after_s" && a.Value.Kind() == slog.KindInt64 {
			after = max(after, time.Duration(a.Value.Int64())*time.Second)
		}
	}
	c.mu.Lock()
	c.until = c.clock.Now().Add(after)
	until := c.until
	c.mu.Unlock()
	observability.Degraded(ctx, observability.MarketHistoryCooldown, slog.String("poller", poller),
		slog.Time("until", until), slog.Int("retry_after_s", int(after/time.Second)))
	return true
}
