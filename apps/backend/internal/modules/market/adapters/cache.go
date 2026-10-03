package adapters

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
)

const (
	shortTTL   = 120 * time.Second
	longTTL    = 15 * time.Minute
	loadBudget = 10 * time.Second
	cacheLimit = 4096
)

type pageBox struct{ v app.Page }

type chartBox struct{ v app.Chart }

type slot struct {
	expires time.Time
	value   any
}

type Cache struct {
	mu     sync.Mutex
	items  map[string]slot
	swept  time.Time
	flight singleflight.Group
	clock  clock.Clock
}

func NewCache(c clock.Clock) *Cache {
	return &Cache{items: map[string]slot{}, clock: c}
}

func (c *Cache) serve(
	ctx context.Context, key string, ttl time.Duration, load func(context.Context) (any, error),
) (any, error) {
	if err := gone(ctx); err != nil {
		return nil, err
	}
	return c.once(ctx, key, ttl, load)
}

func (c *Cache) once(
	ctx context.Context, key string, ttl time.Duration, load func(context.Context) (any, error),
) (any, error) {
	if value, ok := c.hit(key); ok {
		return value, nil
	}
	ch := c.flight.DoChan(key, func() (any, error) {
		if value, ok := c.hit(key); ok {
			return value, nil
		}
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), loadBudget)
		defer cancel()
		value, loadErr := load(loadCtx)
		if loadErr != nil {
			return nil, loadErr
		}
		c.keep(key, value, ttl)
		return value, nil
	})
	var got any
	var err error
	select {
	case <-ctx.Done():
	case res := <-ch:
		got, err = res.Val, res.Err
	}
	if cerr := gone(ctx); cerr != nil {
		return nil, cerr
	}
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "market.cache")
	}
	return got, nil
}

func gone(ctx context.Context) error {
	err := ctx.Err()
	if err == nil {
		return nil
	}
	return errs.Wrap(err, errs.CodeInternal, "market.cache")
}

func (c *Cache) hit(key string) (any, bool) {
	now := c.clock.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	item, found := c.items[key]
	if !found {
		return nil, false
	}
	if !now.Before(item.expires) {
		delete(c.items, key)
		return nil, false
	}
	return item.value, true
}

func (c *Cache) keep(key string, value any, ttl time.Duration) {
	now := c.clock.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sweep(now)
	if len(c.items) >= cacheLimit {
		return
	}
	c.items[key] = slot{expires: now.Add(ttl), value: value}
}

func (c *Cache) sweep(now time.Time) {
	if now.Sub(c.swept) < time.Second {
		return
	}
	c.swept = now
	for key, item := range c.items {
		if !now.Before(item.expires) {
			delete(c.items, key)
		}
	}
}

type CachedList struct {
	next  app.Lister
	cache *Cache
}

func CacheList(next app.Lister, cache *Cache) *CachedList {
	return &CachedList{next: next, cache: cache}
}

func (c *CachedList) Handle(ctx context.Context, req app.ListRequest) (app.Page, error) {
	got, err := c.cache.serve(ctx, listKey(req), shortTTL, func(ctx context.Context) (any, error) {
		page, loadErr := c.next.Handle(ctx, req)
		if loadErr != nil {
			return nil, loadErr
		}
		return pageBox{v: page}, nil
	})
	if err != nil {
		return app.Page{}, err
	}
	return got.(pageBox).v, nil
}

func listKey(req app.ListRequest) string {
	return fmt.Sprintf("%q\x00%q\x00%d\x00%q", req.Q, req.Filter, req.Limit, req.Cursor)
}

type CachedChart struct {
	next  app.Charter
	cache *Cache
}

func CacheChart(next app.Charter, cache *Cache) *CachedChart {
	return &CachedChart{next: next, cache: cache}
}

func (c *CachedChart) Handle(ctx context.Context, symbol, raw string) (app.Chart, error) {
	span, err := domain.ParseChartRange(raw)
	if err != nil {
		return app.Chart{}, err
	}
	ttl := longTTL
	if span == domain.Chart1D {
		ttl = shortTTL
	}
	got, err := c.cache.serve(ctx, chartKey(symbol, span), ttl, func(ctx context.Context) (any, error) {
		chart, loadErr := c.next.Handle(ctx, symbol, string(span))
		if loadErr != nil {
			return nil, loadErr
		}
		return chartBox{v: chart}, nil
	})
	if err != nil {
		return app.Chart{}, err
	}
	return got.(chartBox).v, nil
}

func chartKey(symbol string, span domain.ChartRange) string {
	return "chart\x00" + symbol + "\x00" + string(span)
}

var _ app.Lister = (*CachedList)(nil)

var _ app.Charter = (*CachedChart)(nil)
