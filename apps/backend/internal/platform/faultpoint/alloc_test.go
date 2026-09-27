package faultpoint_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
)

func TestHit_allocatesNothing(t *testing.T) {
	plain := t.Context()
	armed := faultpoint.Armed(plain, faultpoint.AfterPublish)
	allocs := testing.AllocsPerRun(1000, func() {
		faultpoint.Hit(plain, faultpoint.BeforeCommit)
		faultpoint.Hit(armed, faultpoint.BeforeCommit)
	})
	if allocs != 0 {
		t.Fatalf("Hit allocates %.1f times per call pair, want 0", allocs)
	}
}

func BenchmarkHit(b *testing.B) {
	ctx := faultpoint.Armed(b.Context(), faultpoint.AfterPublish)
	b.ReportAllocs()
	for b.Loop() {
		faultpoint.Hit(ctx, faultpoint.BeforeCommit)
	}
}
