package concurrency_test

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/concurrency"
)

var errBoom = errs.New(errs.CodeInternal, "test.boom")

func feed(items ...int) <-chan int {
	in := make(chan int, len(items))
	for _, v := range items {
		in <- v
	}
	close(in)
	return in
}

func double(_ context.Context, v int) (int, error) { return v * 2, nil }

func failOdd(_ context.Context, v int) (int, error) {
	if v%2 == 1 {
		return 0, errBoom
	}
	return v * 2, nil
}

func drainPool(out <-chan int, errc <-chan error, readDelay []time.Duration) (vals []int, fails []error) {
	for out != nil || errc != nil {
		if readDelay != nil {
			<-clock.Real{}.After(readDelay[(len(vals)+len(fails))%len(readDelay)])
		}
		select {
		case v, ok := <-out:
			if !ok {
				out = nil
				continue
			}
			vals = append(vals, v)
		case err, ok := <-errc:
			if !ok {
				errc = nil
				continue
			}
			fails = append(fails, err)
		}
	}
	return vals, fails
}

func assertPanics(t *testing.T, want string, call func()) {
	t.Helper()
	defer func() {
		got, _ := recover().(string)
		if got != want {
			t.Fatalf("panic = %q, want %q", got, want)
		}
	}()
	call()
}

func TestPoolDeliversEveryValueAndErrorThenClosesBothChannels(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		out, errc := concurrency.Pool(t.Context(), 3, feed(1, 2, 3, 4, 5, 6), failOdd)
		vals, fails := drainPool(out, errc, nil)
		slices.Sort(vals)
		if !slices.Equal(vals, []int{4, 8, 12}) {
			t.Fatalf("out = %v, want [4 8 12]", vals)
		}
		if len(fails) != 3 || !errors.Is(fails[0], errBoom) {
			t.Fatalf("fails = %v, want three errBoom", fails)
		}
	})
}

func TestPoolExitsWhenCancelledWhileBlockedSending(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		in := make(chan int, 2)
		in <- 1
		in <- 2
		out, errc := concurrency.Pool(ctx, 2, in, failOdd)
		synctest.Wait()
		cancel()
		synctest.Wait()
		vals, fails := drainPool(out, errc, nil)
		if len(vals)+len(fails) != 0 {
			t.Fatalf("delivered %v %v after cancel with no consumer", vals, fails)
		}
	})
}

func TestPoolExitsWhenCancelledWhileBlockedReceiving(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		out, errc := concurrency.Pool(ctx, 4, make(chan int), double)
		synctest.Wait()
		cancel()
		if _, ok := <-out; ok {
			t.Fatal("out delivered a value with no input")
		}
		if _, ok := <-errc; ok {
			t.Fatal("errs delivered a value with no input")
		}
	})
}

func TestPoolBlocksProducersBehindAStalledConsumer(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var completed atomic.Int32
		fn := func(ctx context.Context, v int) (int, error) {
			defer completed.Add(1)
			return failOdd(ctx, v)
		}
		out, errc := concurrency.Pool(t.Context(), 2, feed(1, 2, 3, 4, 5, 6, 7, 8, 9, 10), fn)
		synctest.Wait()
		if got := completed.Load(); got != 2 {
			t.Fatalf("fn completed %d times before the consumer read, want 2", got)
		}
		vals, fails := drainPool(out, errc, nil)
		slices.Sort(vals)
		if !slices.Equal(vals, []int{4, 8, 12, 16, 20}) {
			t.Fatalf("out = %v, want [4 8 12 16 20]", vals)
		}
		if len(fails) != 5 {
			t.Fatalf("got %d errors, want 5", len(fails))
		}
		if got := completed.Load(); got != 10 {
			t.Fatalf("fn completed %d times, want 10", got)
		}
	})
}

func TestPoolPanicsOnNonPositiveWorkers(t *testing.T) {
	t.Parallel()
	assertPanics(t, "concurrency.Pool: workers must be > 0, got 0", func() {
		concurrency.Pool(t.Context(), 0, feed(), double)
	})
	assertPanics(t, "concurrency.Pool: workers must be > 0, got -3", func() {
		concurrency.Pool(t.Context(), -3, feed(), double)
	})
}

func TestStagePreservesOrderAndClosesOutput(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		fn := func(ctx context.Context, v int) (int, error) {
			<-clock.Real{}.After(time.Duration(10-v) * time.Millisecond)
			return failOdd(ctx, v)
		}
		out := concurrency.Stage(t.Context(), feed(1, 2, 3, 4, 5), 1, fn)
		var got []concurrency.Result[int]
		for r := range out {
			got = append(got, r)
		}
		want := []concurrency.Result[int]{{Err: errBoom}, {Val: 4}, {Err: errBoom}, {Val: 8}, {Err: errBoom}}
		if !slices.EqualFunc(got, want, func(a, b concurrency.Result[int]) bool {
			return a.Val == b.Val && errors.Is(a.Err, b.Err)
		}) {
			t.Fatalf("results = %v, want %v", got, want)
		}
	})
}

func TestStageExitsWhenCancelledWhileBlockedSending(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		in := make(chan int, 3)
		in <- 1
		in <- 2
		in <- 3
		out := concurrency.Stage(ctx, in, 1, double)
		synctest.Wait()
		cancel()
		synctest.Wait()
		var n int
		for range out {
			n++
		}
		if n != 1 {
			t.Fatalf("stage delivered %d results with no consumer, want only the one its buffer held", n)
		}
	})
}

func TestStageExitsWhenCancelledWhileBlockedReceiving(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		out := concurrency.Stage(ctx, make(chan int), 2, double)
		synctest.Wait()
		cancel()
		if r, ok := <-out; ok {
			t.Fatalf("stage delivered %v with no input", r)
		}
	})
}

func TestStagePanicsOnNonPositiveBuf(t *testing.T) {
	t.Parallel()
	assertPanics(t, "concurrency.Stage: buf must be > 0, got 0", func() {
		concurrency.Stage(t.Context(), feed(), 0, double)
	})
	assertPanics(t, "concurrency.Stage: buf must be > 0, got -1", func() {
		concurrency.Stage(t.Context(), feed(), -1, double)
	})
}

func TestFanOutReturnsResultsInInputOrderWithFewerWorkersThanItems(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		fn := func(_ context.Context, v int) (int, error) {
			<-clock.Real{}.After(time.Duration(10-v) * time.Millisecond)
			return v * v, nil
		}
		got, err := concurrency.FanOut(t.Context(), 3, []int{1, 2, 3, 4, 5, 6, 7, 8}, fn)
		if err != nil || !slices.Equal(got, []int{1, 4, 9, 16, 25, 36, 49, 64}) {
			t.Fatalf("FanOut = %v, %v", got, err)
		}
	})
}

func TestFanOutCancelsInFlightAndSkipsUnstartedOnFirstError(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ran := make([]bool, 6)
		causes := make([]error, 6)
		fn := func(ctx context.Context, i int) (int, error) {
			ran[i] = true
			if i == 0 {
				<-clock.Real{}.After(time.Second)
				return 0, errBoom
			}
			<-ctx.Done()
			causes[i] = context.Cause(ctx)
			return 0, causes[i]
		}
		got, err := concurrency.FanOut(t.Context(), 3, []int{0, 1, 2, 3, 4, 5}, fn)
		if got != nil || !errors.Is(err, errBoom) {
			t.Fatalf("FanOut = %v, %v; want nil, errBoom", got, err)
		}
		for i := 1; i <= 2; i++ {
			if !errors.Is(causes[i], errBoom) {
				t.Fatalf("in-flight item %d saw cause %v, want errBoom", i, causes[i])
			}
		}
		if !slices.Equal(ran, []bool{true, true, true, false, false, false}) {
			t.Fatalf("ran = %v, want only the first three", ran)
		}
	})
}

func TestFanOutReturnsParentCauseWhenParentIsCancelled(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		parentCause := errs.New(errs.CodeInternal, "test.parent")
		ctx, cancel := context.WithCancelCause(t.Context())
		fn := func(ctx context.Context, _ int) (int, error) {
			<-ctx.Done()
			return 0, context.Cause(ctx)
		}
		done := make(chan struct{})
		var got []int
		var err error
		go func() {
			defer close(done)
			got, err = concurrency.FanOut(ctx, 2, []int{1, 2, 3}, fn)
		}()
		synctest.Wait()
		cancel(parentCause)
		<-done
		if got != nil || !errors.Is(err, parentCause) {
			t.Fatalf("FanOut = %v, %v; want nil, parent cause", got, err)
		}
	})
}

func TestFanOutEmptyItemsReturnsEmptySlice(t *testing.T) {
	t.Parallel()
	fn := func(context.Context, int) (int, error) {
		t.Fatal("fn ran with no items")
		return 0, nil
	}
	got, err := concurrency.FanOut(t.Context(), 4, nil, fn)
	if got == nil || len(got) != 0 || err != nil {
		t.Fatalf("FanOut(nil) = %#v, %v; want empty non-nil slice", got, err)
	}
}

func TestFanOutPanicsOnNonPositiveLimit(t *testing.T) {
	t.Parallel()
	assertPanics(t, "concurrency.FanOut: limit must be > 0, got 0", func() {
		_, _ = concurrency.FanOut(t.Context(), 0, []int{1}, double)
	})
	assertPanics(t, "concurrency.FanOut: limit must be > 0, got -7", func() {
		_, _ = concurrency.FanOut(t.Context(), -7, []int{1}, double)
	})
}
