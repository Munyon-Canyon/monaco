package concurrency_test

import (
	"context"
	"errors"
	"math/rand/v2"
	"slices"
	"strconv"
	"testing"
	"testing/synctest"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/concurrency"
)

const (
	jitterSeeds = 1000
	jitterItems = 16
)

type jitter struct {
	r *rand.Rand
}

func newJitter(seed uint64) jitter {
	return jitter{r: rand.New(rand.NewPCG(seed, seed))}
}

func (j jitter) delays() []time.Duration {
	d := make([]time.Duration, jitterItems)
	for i := range d {
		d[i] = time.Duration(j.r.IntN(10)) * time.Millisecond
	}
	return d
}

func (j jitter) bound(n int) int { return 1 + j.r.IntN(n) }

func wait(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-clock.Real{}.After(d):
	}
}

func items(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

func TestPoolJitterDeliversTheExactMultiset(t *testing.T) {
	t.Parallel()
	for seed := range uint64(jitterSeeds) {
		t.Run(strconv.FormatUint(seed, 10), func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) { poolSeed(t, seed) })
		})
	}
}

func poolSeed(t *testing.T, seed uint64) {
	t.Helper()
	j := newJitter(seed)
	workers, fnDelay, readDelay := j.bound(4), j.delays(), j.delays()
	failAt := j.bound(jitterItems)
	fn := func(ctx context.Context, v int) (int, error) {
		wait(ctx, fnDelay[v])
		if v%failAt == 0 {
			return 0, errBoom
		}
		return v, nil
	}
	var want []int
	wantErr := 0
	for i := range jitterItems {
		if i%failAt == 0 {
			wantErr++
		} else {
			want = append(want, i)
		}
	}
	out, errc := concurrency.Pool(t.Context(), workers, feed(items(jitterItems)...), fn)
	vals, fails := drainPool(out, errc, readDelay)
	slices.Sort(vals)
	if !slices.Equal(vals, want) || len(fails) != wantErr {
		t.Fatalf("seed %d: out = %v (%d errors), want %v (%d errors)", seed, vals, len(fails), want, wantErr)
	}
	for _, err := range fails {
		if !errors.Is(err, errBoom) {
			t.Fatalf("seed %d: err = %v, want errBoom", seed, err)
		}
	}
}

func TestStageJitterPreservesOrder(t *testing.T) {
	t.Parallel()
	for seed := range uint64(jitterSeeds) {
		t.Run(strconv.FormatUint(seed, 10), func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) { stageSeed(t, seed) })
		})
	}
}

func stageSeed(t *testing.T, seed uint64) {
	t.Helper()
	j := newJitter(seed)
	buf, fnDelay, readDelay := j.bound(4), j.delays(), j.delays()
	fn := func(ctx context.Context, v int) (int, error) {
		wait(ctx, fnDelay[v])
		return v * 2, nil
	}
	got := make([]int, 0, jitterItems)
	for r := range concurrency.Stage(t.Context(), feed(items(jitterItems)...), buf, fn) {
		<-clock.Real{}.After(readDelay[len(got)%jitterItems])
		if r.Err != nil {
			t.Fatalf("seed %d: result %d carried %v", seed, len(got), r.Err)
		}
		got = append(got, r.Val)
	}
	if want := doubles(jitterItems); !slices.Equal(got, want) {
		t.Fatalf("seed %d: out = %v, want %v", seed, got, want)
	}
}

func TestFanOutJitterPreservesOrderOrFailsWhole(t *testing.T) {
	t.Parallel()
	for seed := range uint64(jitterSeeds) {
		t.Run(strconv.FormatUint(seed, 10), func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) { fanOutSeed(t, seed) })
		})
	}
}

func fanOutSeed(t *testing.T, seed uint64) {
	t.Helper()
	j := newJitter(seed)
	limit, fnDelay := j.bound(jitterItems), j.delays()
	failAt := j.bound(jitterItems+1) - 2
	fn := func(ctx context.Context, v int) (int, error) {
		wait(ctx, fnDelay[v])
		if v == failAt {
			return 0, errBoom
		}
		return v * 2, nil
	}
	got, err := concurrency.FanOut(t.Context(), limit, items(jitterItems), fn)
	if failAt >= 0 {
		if got != nil || !errors.Is(err, errBoom) {
			t.Fatalf("seed %d: FanOut = %v, %v; want nil, errBoom", seed, got, err)
		}
		return
	}
	if want := doubles(jitterItems); err != nil || !slices.Equal(got, want) {
		t.Fatalf("seed %d: FanOut = %v, %v; want %v, nil", seed, got, err, want)
	}
}

func doubles(n int) []int {
	out := items(n)
	for i := range out {
		out[i] *= 2
	}
	return out
}
