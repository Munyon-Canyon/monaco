package testkit

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

var errSocketTimeout = errors.New("write failed: write tcp 127.0.0.1:59336->127.0.0.1:54323: i/o timeout")

func lazyPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), "postgres://probe@127.0.0.1:1/probe")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func runProbe(t *testing.T, read func(ctx context.Context, calls int) error, waitForError bool) []string {
	t.Helper()
	count := 0
	p := startProbe(t.Context(), lazyPool(t), func(ctx context.Context) error {
		count++
		return read(ctx, count)
	})
	if waitForError {
		_ = p.group.Wait()
	}
	rec := &recordingTB{TB: t}
	p.Check(rec)
	return rec.failures
}

func TestProbeCheckPassesWhenALateReadTimesOutAfterTheProbeIsCancelled(t *testing.T) {
	t.Parallel()
	failures := runProbe(t, func(ctx context.Context, calls int) error {
		if calls == 1 {
			return nil
		}
		<-ctx.Done()
		return errSocketTimeout
	}, false)
	if len(failures) != 0 {
		t.Fatalf("Check failed: %v", failures)
	}
}

func TestProbeCheckFailsWhenAReadErrorsWhileTheCallersHoldConnections(t *testing.T) {
	t.Parallel()
	failures := runProbe(t, func(_ context.Context, calls int) error {
		if calls == 1 {
			return nil
		}
		return errSocketTimeout
	}, true)
	if len(failures) != 1 || !strings.Contains(failures[0], "i/o timeout") ||
		!strings.Contains(failures[0], "pool acquired=") {
		t.Fatalf("Check failures = %v, want one naming the error and the pool stats", failures)
	}
}

func TestProbeCheckFailsWithoutAReadDuringTheHold(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	p := startProbe(ctx, lazyPool(t), func(context.Context) error { return errSocketTimeout })
	rec := &recordingTB{TB: t}
	p.Check(rec)
	if len(rec.failures) != 1 || !strings.Contains(rec.failures[0], "unrelated reads = 0") ||
		!strings.Contains(rec.failures[0], "pool acquired=") {
		t.Fatalf("Check failures = %v, want one with zero reads and the pool stats", rec.failures)
	}
}
