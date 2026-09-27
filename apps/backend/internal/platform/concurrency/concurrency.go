package concurrency

import (
	"context"
	"fmt"
	"sync"
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
	panic("unimplemented")
}

func mustPositive(helper, name string, n int) {
	if n <= 0 {
		panic(fmt.Sprintf("concurrency.%s: %s must be > 0, got %d", helper, name, n))
	}
}
