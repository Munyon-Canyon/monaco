package db_test

import (
	"bytes"
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func hashA() []byte { return bytes.Repeat([]byte{0xa1}, 32) }

func hashB() []byte { return bytes.Repeat([]byte{0xb2}, 32) }

func storedOK() db.StoredResponse {
	return db.StoredResponse{
		Status: http.StatusCreated,
		Header: http.Header{"Content-Type": {"application/json"}, "X-Thing": {"1", "2"}},
		Body:   []byte(`{"id":"t1"}` + "\n"),
	}
}

func mustBegin(t *testing.T, s *db.IdempotencyStore, actor, key string, hash []byte) db.Claim {
	t.Helper()
	claim, err := s.Begin(t.Context(), actor, key, hash)
	if err != nil {
		t.Fatalf("Begin(%s, %s) = %v", actor, key, err)
	}
	return claim
}

func TestIdempotencyStore_secondBeginSeesInFlightThenTheStoredResponse(t *testing.T) {
	t.Parallel()
	clk := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	s := db.NewIdempotencyStore(testkit.DB(t), clk)

	if c := mustBegin(t, s, "user:u1", "k1", hashA()); c.Outcome != db.ClaimOwned {
		t.Fatalf("first Begin = %+v, want ClaimOwned", c)
	}
	if c := mustBegin(t, s, "user:u1", "k1", hashA()); c.Outcome != db.ClaimInFlight {
		t.Fatalf("Begin while in flight = %+v, want ClaimInFlight", c)
	}
	if c := mustBegin(t, s, "user:u1", "k1", hashB()); c.Outcome != db.ClaimMismatch {
		t.Fatalf("Begin with another hash = %+v, want ClaimMismatch", c)
	}
	if c := mustBegin(t, s, "user:u2", "k1", hashA()); c.Outcome != db.ClaimOwned {
		t.Fatalf("Begin for another actor = %+v, want ClaimOwned: keys are scoped per actor", c)
	}
}

func TestIdempotencyStore_beginAfterCompleteReturnsTheStoredResponse(t *testing.T) {
	t.Parallel()
	s := db.NewIdempotencyStore(testkit.DB(t), testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)))
	mustBegin(t, s, "user:u1", "k1", hashA())
	if err := s.Complete(t.Context(), "user:u1", "k1", storedOK()); err != nil {
		t.Fatal(err)
	}
	c := mustBegin(t, s, "user:u1", "k1", hashA())
	want := storedOK()
	if c.Outcome != db.ClaimCompleted || c.Response.Status != want.Status ||
		!bytes.Equal(c.Response.Body, want.Body) || len(c.Response.Header) != len(want.Header) ||
		c.Response.Header.Get("Content-Type") != "application/json" ||
		len(c.Response.Header.Values("X-Thing")) != 2 || c.Response.Header.Values("X-Thing")[1] != "2" {
		t.Fatalf("Begin after Complete = %+v, want the stored %+v", c, want)
	}
	if c := mustBegin(t, s, "user:u1", "k1", hashB()); c.Outcome != db.ClaimMismatch {
		t.Fatalf("Begin with another hash after Complete = %+v, want ClaimMismatch", c)
	}
}

func TestIdempotencyStore_releaseHandsTheKeyBackAndCompleteNeedsAnInFlightRow(t *testing.T) {
	t.Parallel()
	s := db.NewIdempotencyStore(testkit.DB(t), testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)))
	mustBegin(t, s, "user:u1", "k1", hashA())
	if err := s.Release(t.Context(), "user:u1", "k1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Release(t.Context(), "user:u1", "k1"); err != nil {
		t.Fatalf("Release of an absent key = %v, want nil: release converges", err)
	}
	if c := mustBegin(t, s, "user:u1", "k1", hashB()); c.Outcome != db.ClaimOwned {
		t.Fatalf("Begin after Release = %+v, want ClaimOwned with any hash", c)
	}
	if err := s.Complete(t.Context(), "user:u1", "k1", storedOK()); err != nil {
		t.Fatal(err)
	}
	if err := s.Release(t.Context(), "user:u1", "k1"); err != nil {
		t.Fatal(err)
	}
	if c := mustBegin(t, s, "user:u1", "k1", hashB()); c.Outcome != db.ClaimCompleted {
		t.Fatalf("Begin after Release of a completed key = %+v, want ClaimCompleted: only in-flight rows drop", c)
	}
	if err := s.Complete(t.Context(), "user:u1", "k1", storedOK()); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Complete twice = %v, want internal", err)
	}
	if err := s.Complete(t.Context(), "user:u1", "never-begun", storedOK()); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Complete without Begin = %v, want internal", err)
	}
}

func TestIdempotencyStore_deleteBeforeExpiresOldKeysOnly(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	start := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	clk := testkit.NewClock(start)
	s := db.NewIdempotencyStore(pool, clk)
	mustBegin(t, s, "user:u1", "old", hashA())
	if err := s.Complete(t.Context(), "user:u1", "old", storedOK()); err != nil {
		t.Fatal(err)
	}
	clk.Advance(25 * time.Hour)
	mustBegin(t, s, "user:u1", "new", hashA())

	n, err := sqlc.New(pool).DeleteIdempotencyKeysBefore(t.Context(), clk.Now().Add(-24*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("DeleteIdempotencyKeysBefore = %d, %v, want 1 row", n, err)
	}
	if c := mustBegin(t, s, "user:u1", "old", hashB()); c.Outcome != db.ClaimOwned {
		t.Fatalf("Begin on an expired key = %+v, want ClaimOwned", c)
	}
	if c := mustBegin(t, s, "user:u1", "new", hashA()); c.Outcome != db.ClaimInFlight {
		t.Fatalf("Begin on a fresh key = %+v, want ClaimInFlight", c)
	}
}

func TestIdempotencyStore_concurrentBeginsOwnTheKeyExactlyOnce(t *testing.T) {
	t.Parallel()
	s := db.NewIdempotencyStore(testkit.DB(t), testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)))
	const n = 4
	outcomes := make(chan db.ClaimOutcome, n)
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			c, err := s.Begin(t.Context(), "user:u1", "k1", hashA())
			if err != nil {
				t.Error(err)
			}
			outcomes <- c.Outcome
		})
	}
	wg.Wait()
	close(outcomes)
	owned, inFlight := 0, 0
	for o := range outcomes {
		switch o {
		case db.ClaimOwned:
			owned++
		case db.ClaimInFlight:
			inFlight++
		case db.ClaimMismatch, db.ClaimCompleted:
			t.Errorf("outcome %d, want owned or in flight", o)
		}
	}
	if owned != 1 || inFlight != n-1 {
		t.Fatalf("owned = %d, in flight = %d, want 1 and %d", owned, inFlight, n-1)
	}
}

func TestIdempotencyStore_closedPoolIsACodedError(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	s := db.NewIdempotencyStore(pool, testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)))
	pool.Close()
	ctx := context.Background()
	if _, err := s.Begin(ctx, "user:u1", "k1", hashA()); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Begin = %v, want internal", err)
	}
	if err := s.Complete(ctx, "user:u1", "k1", storedOK()); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Complete = %v, want internal", err)
	}
	if err := s.Release(ctx, "user:u1", "k1"); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Release = %v, want internal", err)
	}
}
