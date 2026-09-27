package concurrency

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

type Result[Out any] struct {
	Val Out
	Err error
}

func Pool[In, Out any](
	ctx context.Context, workers int, in <-chan In, fn func(context.Context, In) (Out, error),
) (<-chan Out, <-chan error) {
	mustPositive("Pool", "workers", workers)
	out, errc := make(chan Out), make(chan error)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for {
				select {
				case <-ctx.Done():
					return
				case v, ok := <-in:
					if !ok {
						return
					}
					res, err := fn(ctx, v)
					if err != nil {
						select {
						case <-ctx.Done():
						case errc <- err:
						}
						continue
					}
					select {
					case <-ctx.Done():
					case out <- res:
					}
				}
			}
		})
	}
	go func() {
		wg.Wait()
		close(out)
		close(errc)
	}()
	return out, errc
}

func Stage[In, Out any](
	ctx context.Context, in <-chan In, buf int, fn func(context.Context, In) (Out, error),
) <-chan Result[Out] {
	mustPositive("Stage", "buf", buf)
	out := make(chan Result[Out], buf)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case v, ok := <-in:
				if !ok {
					return
				}
				res, err := fn(ctx, v)
				select {
				case <-ctx.Done():
					return
				case out <- Result[Out]{Val: res, Err: err}:
				}
			}
		}
	}()
	return out
}

func FanOut[T, R any](
	ctx context.Context, limit int, items []T, fn func(context.Context, T) (R, error),
) ([]R, error) {
	mustPositive("FanOut", "limit", limit)
	out := make([]R, len(items))
	if len(items) == 0 {
		return out, nil
	}
	gctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	var next atomic.Int64
	var wg sync.WaitGroup
	for range min(limit, len(items)) {
		wg.Go(func() {
			for gctx.Err() == nil {
				i := int(next.Add(1) - 1)
				if i >= len(items) {
					return
				}
				res, err := fn(gctx, items[i])
				if err != nil {
					cancel(err)
					return
				}
				out[i] = res
			}
		})
	}
	wg.Wait()
	if err := context.Cause(gctx); err != nil {
		return nil, err
	}
	return out, nil
}

func mustPositive(helper, name string, n int) {
	if n <= 0 {
		panic(fmt.Sprintf("concurrency.%s: %s must be > 0, got %d", helper, name, n))
	}
}
