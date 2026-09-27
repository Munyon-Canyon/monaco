package concurrency_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/concurrency"
)

const fanOutAllocBudget = 14

func BenchmarkPool(b *testing.B) {
	in := items(64)
	for b.Loop() {
		out, errc := concurrency.Pool(context.Background(), 4, feed(in...), double)
		drainPool(out, errc, nil)
	}
}

func BenchmarkStage(b *testing.B) {
	in := items(64)
	for b.Loop() {
		for range concurrency.Stage(context.Background(), feed(in...), 8, double) {
		}
	}
}

func BenchmarkFanOut(b *testing.B) {
	in := items(8)
	run := func() {
		if _, err := concurrency.FanOut(context.Background(), 4, in, double); err != nil {
			b.Fatal(err)
		}
	}
	if allocs := testing.AllocsPerRun(200, run); allocs > fanOutAllocBudget {
		b.Fatalf("FanOut over 8 items with limit 4 allocates %.0f per run, budget %d", allocs, fanOutAllocBudget)
	}
	for b.Loop() {
		run()
	}
}
