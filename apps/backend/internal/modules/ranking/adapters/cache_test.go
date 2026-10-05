package adapters_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
)

func TestLRU_evictsTheOldestEntryAtCapacityPlusOne(t *testing.T) {
	t.Parallel()
	lru := adapters.NewLRU[int, string](512)
	for i := range 513 {
		lru.Add(i, "v")
	}
	if lru.Get(0) != nil || lru.Len() != 512 {
		t.Fatalf("after 513 adds: entry 0 = %v, len = %d, want evicted and 512", lru.Get(0), lru.Len())
	}
	if lru.Get(512) == nil {
		t.Fatal("newest entry was evicted")
	}
}

func TestLRU_readsAndRewritesRefreshRecency(t *testing.T) {
	t.Parallel()
	lru := adapters.NewLRU[string, int](2)
	lru.Add("a", 1)
	lru.Add("b", 2)
	if v := lru.Get("a"); v == nil || *v != 1 {
		t.Fatalf("Get(a) = %v", v)
	}
	lru.Add("c", 3)
	if lru.Get("b") != nil {
		t.Fatal("b survived although a was read after it")
	}
	lru.Add("a", 10)
	lru.Add("d", 4)
	if v := lru.Get("a"); v == nil || *v != 10 {
		t.Fatalf("Get(a) = %v, want the rewritten value kept fresh", v)
	}
	if lru.Get("c") != nil {
		t.Fatal("c survived although a was rewritten after it")
	}
}

func TestCache_twentyConcurrentMissesRunOneLoad(t *testing.T) {
	t.Parallel()
	cache := adapters.NewCache[string, int](4)
	var loads atomic.Int64
	started, release := make(chan struct{}), make(chan struct{})
	load := func(ctx context.Context) (int, error) {
		if loads.Add(1) == 1 {
			close(started)
		}
		<-release
		return 7, ctx.Err()
	}
	var group errgroup.Group
	for range 20 {
		group.Go(func() error {
			got, err := cache.Load(t.Context(), "k", load)
			if err != nil || *got != 7 {
				t.Errorf("Load = %v, %v", got, err)
			}
			return nil
		})
	}
	<-started
	close(release)
	if err := group.Wait(); err != nil {
		t.Fatal(err)
	}
	if loads.Load() != 1 {
		t.Fatalf("loads = %d, want 1", loads.Load())
	}
}

func TestCache_doesNotKeepFailuresAndLoadsOutliveTheCaller(t *testing.T) {
	t.Parallel()
	cache := adapters.NewCache[string, int](4)
	failure := errs.New(errs.CodeInternal, "test")
	fail := func(context.Context) (int, error) { return 0, failure }
	if _, err := cache.Load(t.Context(), "k", fail); !errors.Is(err, failure) {
		t.Fatalf("Load err = %v, want the load's error", err)
	}
	gone, cancel := context.WithCancel(t.Context())
	cancel()
	got, err := cache.Load(gone, "k", func(ctx context.Context) (int, error) { return 1, ctx.Err() })
	if err != nil || *got != 1 {
		t.Fatalf("Load after a failure = %v, %v, want a fresh load that ignores the caller's cancel", got, err)
	}
}

func TestCache_aWaitingCallerStopsWhenItsContextEnds(t *testing.T) {
	t.Parallel()
	cache := adapters.NewCache[string, int](4)
	started, release := make(chan struct{}), make(chan struct{})
	var group errgroup.Group
	group.Go(func() error {
		_, err := cache.Load(t.Context(), "k", func(context.Context) (int, error) {
			close(started)
			<-release
			return 1, nil
		})
		return err
	})
	<-started
	gone, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := cache.Load(gone, "k", nil); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("waiter err = %v, want internal", err)
	}
	close(release)
	if err := group.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestPageCache_returnsThePageOrTheLoadError(t *testing.T) {
	t.Parallel()
	pages := adapters.PageCache{Cache: adapters.NewCache[app.PageKey, domain.BoardPage](2)}
	key := app.PageKey{Board: "cabals", Limit: 20}
	three := domain.BoardPage{Rows: make([]domain.Entry, 3)}
	load := func(context.Context) (domain.BoardPage, error) { return three, nil }
	if page, err := pages.Load(t.Context(), key, load); err != nil || len(page.Rows) != 3 {
		t.Fatalf("Load = %+v, %v, want the loaded page", page, err)
	}
	broken := errs.New(errs.CodeInternal, "t")
	failing := func(context.Context) (domain.BoardPage, error) { return domain.BoardPage{}, broken }
	if _, err := pages.Load(t.Context(), app.PageKey{Board: "people"}, failing); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Load err = %v, want internal", err)
	}
}
