package adapters

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
)

const (
	historyShortTTL = 15 * time.Minute
	historyLongTTL  = time.Hour
	historyMissTTL  = 24 * time.Hour
)

type historyBox struct {
	history app.History
	missing bool
}

type CachedHistory struct {
	next  app.PriceHistory
	cache *Cache
}

func CacheHistory(next app.PriceHistory, cache *Cache) *CachedHistory {
	return &CachedHistory{next: next, cache: cache}
}

func (h *CachedHistory) Configured() bool { return h.next.Configured() }

func (h *CachedHistory) MarketChart(ctx context.Context, mint domain.Mint, days int) (app.History, error) {
	key := fmt.Sprintf("history\x00%s\x00%d", mint, days)
	got, err, _ := h.cache.flight.Do(key, func() (any, error) {
		if value, ok := h.cache.hit(key); ok {
			return value, nil
		}
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), loadBudget)
		defer cancel()
		history, loadErr := h.next.MarketChart(loadCtx, mint, days)
		switch {
		case errs.CodeOf(loadErr) == errs.CodeNotFound:
			h.cache.keep(key, historyBox{missing: true}, historyMissTTL)
			return historyBox{missing: true}, nil
		case loadErr != nil:
			return nil, loadErr
		}
		box := historyBox{history: history}
		h.cache.keep(key, box, historyTTL(days))
		return box, nil
	})
	if err != nil {
		return app.History{}, errs.Wrap(err, errs.CodeOf(err), "market.CachedHistory",
			slog.String("mint", mint.String()))
	}
	box := got.(historyBox)
	if box.missing {
		return app.History{}, errs.New(errs.CodeNotFound, "market.CachedHistory", slog.String("mint", mint.String()))
	}
	return box.history, nil
}

func historyTTL(days int) time.Duration {
	if days == 1 {
		return historyShortTTL
	}
	return historyLongTTL
}

var _ app.PriceHistory = (*CachedHistory)(nil)
