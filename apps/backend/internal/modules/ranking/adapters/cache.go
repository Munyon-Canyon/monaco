package adapters

import (
	"context"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
)

const loadBudget = 10 * time.Second

type node[K comparable, V any] struct {
	key        K
	val        V
	prev, next *node[K, V]
}

type LRU[K comparable, V any] struct {
	max   int
	items map[K]*node[K, V]
	head  node[K, V]
}

func NewLRU[K comparable, V any](size int) *LRU[K, V] {
	l := &LRU[K, V]{max: size, items: make(map[K]*node[K, V], size)}
	l.head.prev, l.head.next = &l.head, &l.head
	return l
}

func (l *LRU[K, V]) Len() int { return len(l.items) }

func (l *LRU[K, V]) Get(key K) *V {
	n, ok := l.items[key]
	if !ok {
		return nil
	}
	l.unlink(n)
	l.pushFront(n)
	val := n.val
	return &val
}

func (l *LRU[K, V]) Add(key K, val V) {
	if n, ok := l.items[key]; ok {
		n.val = val
		l.unlink(n)
		l.pushFront(n)
		return
	}
	n := &node[K, V]{key: key, val: val}
	l.items[key] = n
	l.pushFront(n)
	if len(l.items) > l.max {
		oldest := l.head.prev
		l.unlink(oldest)
		delete(l.items, oldest.key)
	}
}

func (l *LRU[K, V]) unlink(n *node[K, V]) {
	n.prev.next, n.next.prev = n.next, n.prev
}

func (l *LRU[K, V]) pushFront(n *node[K, V]) {
	n.prev, n.next = &l.head, l.head.next
	l.head.next.prev = n
	l.head.next = n
}

type call[V any] struct {
	done chan struct{}
	val  V
	err  error
}

type Cache[K comparable, V any] struct {
	mu    sync.Mutex
	lru   *LRU[K, V]
	calls map[K]*call[V]
}

func NewCache[K comparable, V any](size int) *Cache[K, V] {
	return &Cache[K, V]{lru: NewLRU[K, V](size), calls: map[K]*call[V]{}}
}

func (c *Cache[K, V]) Load(ctx context.Context, key K, load func(context.Context) (V, error)) (*V, error) {
	c.mu.Lock()
	if val := c.lru.Get(key); val != nil {
		c.mu.Unlock()
		return val, nil
	}
	if flight, ok := c.calls[key]; ok {
		c.mu.Unlock()
		return await(ctx, flight)
	}
	flight := &call[V]{done: make(chan struct{})}
	c.calls[key] = flight
	c.mu.Unlock()
	loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), loadBudget)
	defer cancel()
	flight.val, flight.err = load(loadCtx)
	c.mu.Lock()
	if flight.err == nil {
		c.lru.Add(key, flight.val)
	}
	delete(c.calls, key)
	c.mu.Unlock()
	close(flight.done)
	return flight.result()
}

func (f *call[V]) result() (*V, error) {
	if f.err != nil {
		return nil, f.err
	}
	val := f.val
	return &val, nil
}

func await[V any](ctx context.Context, flight *call[V]) (*V, error) {
	select {
	case <-flight.done:
		return flight.result()
	case <-ctx.Done():
		return nil, errs.Wrap(context.Cause(ctx), errs.CodeInternal, "ranking.Cache.Load")
	}
}

type PageCache struct {
	Cache *Cache[app.PageKey, domain.BoardPage]
}

func (p PageCache) Load(
	ctx context.Context, key app.PageKey, load func(context.Context) (domain.BoardPage, error),
) (domain.BoardPage, error) {
	page, err := p.Cache.Load(ctx, key, load)
	if err != nil {
		return domain.BoardPage{}, err
	}
	return *page, nil
}

type HistoryCache struct {
	Cache *Cache[app.HistoryKey, domain.ValueHistory]
}

func (h HistoryCache) Load(
	ctx context.Context, key app.HistoryKey, load func(context.Context) (domain.ValueHistory, error),
) (domain.ValueHistory, error) {
	history, err := h.Cache.Load(ctx, key, load)
	if err != nil {
		return domain.ValueHistory{}, err
	}
	return *history, nil
}
