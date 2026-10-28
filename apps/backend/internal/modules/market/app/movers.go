package app

import (
	"context"
	"encoding/base64"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
)

type moversBook struct {
	mu    sync.Mutex
	q     string
	tick  time.Time
	items []Summary
}

func (l *ListAssets) movers(ctx context.Context, q listQuery, now time.Time) ([]Summary, string, error) {
	ranked, err := l.rankedMovers(ctx, q.q, now)
	if err != nil {
		return nil, "", err
	}
	start := 0
	if q.cursor.ok {
		start = len(ranked)
		for i, s := range ranked {
			if after(s, q.cursor) {
				start = i
				break
			}
		}
	}
	rest := ranked[start:]
	if len(rest) <= int(q.limit) {
		return append([]Summary{}, rest...), "", nil
	}
	items := append([]Summary{}, rest[:q.limit]...)
	return items, encodeMoverCursor(items[len(items)-1]), nil
}

func (l *ListAssets) rankedMovers(ctx context.Context, search string, now time.Time) ([]Summary, error) {
	candidates, err := l.moverCandidates(ctx, search)
	if err != nil {
		return nil, err
	}
	newest, err := l.newest(ctx, mintsOf(candidates), now)
	if err != nil {
		return nil, err
	}
	tick := newestTick(newest)
	if cached, ok := l.ranked.get(search, tick); ok {
		return cached, nil
	}
	loaded, err := l.summaries(ctx, candidates, now)
	if err != nil {
		return nil, err
	}
	ranked := rankMovers(loaded)
	l.ranked.put(search, tick, ranked)
	return ranked, nil
}

func (l *ListAssets) moverCandidates(ctx context.Context, search string) ([]domain.Asset, error) {
	all, err := l.catalog.ListAll(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "market.ListAssets.movers")
	}
	hot, err := hotSet(ctx, l.hot, all)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "market.ListAssets.movers")
	}
	var out []domain.Asset
	for _, a := range all {
		if a.Kind == domain.KindEquity && a.Tradable() && hot[a.Mint.Address()] && matches(a, search) {
			out = append(out, a)
		}
	}
	return out, nil
}

func matches(a domain.Asset, search string) bool {
	if search == "" {
		return true
	}
	needle := strings.ToLower(search)
	return strings.HasPrefix(strings.ToLower(a.Symbol), needle) ||
		strings.Contains(strings.ToLower(a.DisplayName), needle)
}

func newestTick(newest map[string][]domain.Sample) time.Time {
	var tick time.Time
	for _, samples := range newest {
		if len(samples) > 0 && samples[0].ObservedAt.After(tick) {
			tick = samples[0].ObservedAt
		}
	}
	return tick
}

func rankMovers(in []Summary) []Summary {
	out := make([]Summary, 0, len(in))
	for _, s := range in {
		if s.Priced && s.Change != nil {
			out = append(out, s)
		}
	}
	slices.SortFunc(out, func(a, b Summary) int {
		if d := absBps(*b.Change) - absBps(*a.Change); d != 0 {
			return int(d)
		}
		return strings.Compare(a.Asset.Symbol, b.Asset.Symbol)
	})
	return out
}

func absBps(v int32) int64 {
	if v < 0 {
		return -int64(v)
	}
	return int64(v)
}

func after(s Summary, cur pageCursor) bool {
	abs := absBps(*s.Change)
	if abs != int64(cur.move) {
		return abs < int64(cur.move)
	}
	return s.Asset.Symbol > cur.symbol
}

func encodeMoverCursor(last Summary) string {
	head := "m" + strconv.FormatInt(absBps(*last.Change), 10) + "," + last.Asset.Symbol
	raw := head + "\x00" + last.Asset.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeMoverCursor(head string, id uuid.UUID) (pageCursor, error) {
	move, symbol, found := strings.Cut(head[1:], ",")
	abs, err := strconv.ParseInt(move, 10, 32)
	if !found || symbol == "" || err != nil || abs < 0 {
		return pageCursor{}, errs.New(errs.CodeInvalidInput, "market.cursor")
	}
	return pageCursor{ok: true, move: int32(abs), symbol: symbol, id: id}, nil
}

func (b *moversBook) get(q string, tick time.Time) ([]Summary, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.items == nil || b.q != q || !b.tick.Equal(tick) {
		return nil, false
	}
	return b.items, true
}

func (b *moversBook) put(q string, tick time.Time, items []Summary) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.q, b.tick, b.items = q, tick, items
}
