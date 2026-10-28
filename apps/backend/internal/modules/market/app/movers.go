package app

import (
	"cmp"
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

type tickBook struct {
	mu     sync.Mutex
	q      string
	tick   time.Time
	loaded bool
	orders tickOrders
}

type tickOrders struct {
	movers []Summary
	price  []Summary
}

func (l *ListAssets) ranking(ctx context.Context, q listQuery, now time.Time) ([]Summary, string, error) {
	orders, err := l.tickOrders(ctx, q.q, now)
	if err != nil {
		return nil, "", err
	}
	ranked, afterCursor, encode := orders.movers, moverAfter, encodeMoverCursor
	if q.filter != FilterMovers {
		ranked, afterCursor, encode = orders.price, priceAfter, encodePriceCursor
	}
	start := 0
	if q.cursor.ok {
		start = len(ranked)
		for i, s := range ranked {
			if afterCursor(s, q.cursor) {
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
	return items, encode(items[len(items)-1]), nil
}

func (l *ListAssets) tickOrders(ctx context.Context, search string, now time.Time) (tickOrders, error) {
	all, candidates, err := l.equities(ctx, search)
	if err != nil {
		return tickOrders{}, err
	}
	newest, err := l.newest(ctx, mintsOf(candidates), now)
	if err != nil {
		return tickOrders{}, err
	}
	tick := newestTick(newest)
	if cached, ok := l.ranked.get(search, tick); ok {
		return cached, nil
	}
	hot, err := hotSet(ctx, l.hot, all)
	if err != nil {
		return tickOrders{}, errs.Wrap(err, errs.CodeOf(err), "market.ListAssets.movers")
	}
	loaded, err := l.summaries(ctx, candidates, now)
	if err != nil {
		return tickOrders{}, err
	}
	var warm []Summary
	for _, s := range loaded {
		if hot[s.Asset.Mint.Address()] {
			warm = append(warm, s)
		}
	}
	orders := tickOrders{movers: rankMovers(warm), price: rankPrices(loaded)}
	l.ranked.put(search, tick, orders)
	return orders, nil
}

func (l *ListAssets) equities(ctx context.Context, search string) (all, candidates []domain.Asset, err error) {
	all, err = l.catalog.ListAll(ctx)
	if err != nil {
		return nil, nil, errs.Wrap(err, errs.CodeOf(err), "market.ListAssets.movers")
	}
	for _, a := range all {
		if a.Kind == domain.KindEquity && a.Tradable() && matches(a, search) {
			candidates = append(candidates, a)
		}
	}
	return all, candidates, nil
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

func moverAfter(s Summary, cur pageCursor) bool {
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

func rankPrices(in []Summary) []Summary {
	out := slices.Clone(in)
	slices.SortFunc(out, func(a, b Summary) int {
		return comparePrice(
			a.Priced, a.Price.Micros.Uint64(), a.Asset.Symbol,
			b.Priced, b.Price.Micros.Uint64(), b.Asset.Symbol,
		)
	})
	return out
}

func comparePrice(
	aPriced bool, aMicros uint64, aSymbol string, bPriced bool, bMicros uint64, bSymbol string,
) int {
	if aPriced != bPriced {
		if aPriced {
			return -1
		}
		return 1
	}
	if aPriced {
		if d := cmp.Compare(bMicros, aMicros); d != 0 {
			return d
		}
	}
	return strings.Compare(aSymbol, bSymbol)
}

func priceAfter(s Summary, cur pageCursor) bool {
	return comparePrice(s.Priced, s.Price.Micros.Uint64(), s.Asset.Symbol, !cur.unpriced, cur.price, cur.symbol) > 0
}

func encodePriceCursor(last Summary) string {
	price := "-"
	if last.Priced {
		price = strconv.FormatUint(last.Price.Micros.Uint64(), 10)
	}
	raw := "p" + price + "," + last.Asset.Symbol + "\x00" + last.Asset.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodePriceCursor(head string, id uuid.UUID) (pageCursor, error) {
	price, symbol, found := strings.Cut(head[1:], ",")
	if !found || symbol == "" {
		return pageCursor{}, errs.New(errs.CodeInvalidInput, "market.cursor")
	}
	if price == "-" {
		return pageCursor{ok: true, unpriced: true, symbol: symbol, id: id}, nil
	}
	micros, err := strconv.ParseUint(price, 10, 64)
	if err != nil {
		return pageCursor{}, errs.Wrap(err, errs.CodeInvalidInput, "market.cursor")
	}
	return pageCursor{ok: true, price: micros, symbol: symbol, id: id}, nil
}

func (b *tickBook) get(q string, tick time.Time) (tickOrders, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.loaded || b.q != q || !b.tick.Equal(tick) {
		return tickOrders{}, false
	}
	return b.orders, true
}

func (b *tickBook) put(q string, tick time.Time, orders tickOrders) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.q, b.tick, b.orders, b.loaded = q, tick, orders, true
}
