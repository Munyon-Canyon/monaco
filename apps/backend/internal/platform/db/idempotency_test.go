package db_test

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

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

func TestIdempotencyStore_takesOverAnInFlightRowAbandonedForFiveMinutes(t *testing.T) {
	t.Parallel()
	clk := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	s := db.NewIdempotencyStore(testkit.DB(t), clk)
	mustBegin(t, s, "user:u1", "k1", hashA())
	clk.Advance(5 * time.Minute)
	if c := mustBegin(t, s, "user:u1", "k1", hashA()); c.Outcome != db.ClaimInFlight {
		t.Fatalf("Begin at exactly 5m = %+v, want ClaimInFlight", c)
	}
	clk.Advance(time.Second)
	if c := mustBegin(t, s, "user:u1", "k1", hashB()); c.Outcome != db.ClaimMismatch {
		t.Fatalf("stale row with another hash = %+v, want ClaimMismatch: the key stays used", c)
	}
	if c := mustBegin(t, s, "user:u1", "k1", hashA()); c.Outcome != db.ClaimOwned {
		t.Fatalf("Begin after 5m1s = %+v, want ClaimOwned", c)
	}
	if c := mustBegin(t, s, "user:u1", "k1", hashA()); c.Outcome != db.ClaimInFlight {
		t.Fatalf("Begin right after the takeover = %+v, want ClaimInFlight: created_at was reset", c)
	}
	if err := s.Complete(t.Context(), "user:u1", "k1", storedOK()); err != nil {
		t.Fatal(err)
	}
	clk.Advance(time.Hour)
	if c := mustBegin(t, s, "user:u1", "k1", hashA()); c.Outcome != db.ClaimCompleted {
		t.Fatalf("old completed row = %+v, want ClaimCompleted: takeover only touches in-flight rows", c)
	}
}

func TestIdempotencyStore_corruptStoredHeadersAreADecodeFailure(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	s := db.NewIdempotencyStore(pool, testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)))
	mustBegin(t, s, "user:u1", "k1", hashA())
	if err := s.Complete(t.Context(), "user:u1", "k1", storedOK()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(),
		`UPDATE idempotency_keys SET response_headers = '"not a header map"' WHERE key = 'k1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Begin(t.Context(), "user:u1", "k1", hashA()); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("Begin = %v, want decode_failed", err)
	}
}

type fakeDB struct {
	exec func(sql string) (pgconn.CommandTag, error)
	scan error
	row  *sqlc.IdempotencyKey
}

type fakeRow struct {
	err error
	row *sqlc.IdempotencyKey
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*dest[0].(*string) = r.row.ActorKey
	*dest[1].(*string) = r.row.Key
	*dest[2].(*[]byte) = r.row.RequestHash
	*dest[3].(*int16) = r.row.Status
	*dest[7].(*time.Time) = r.row.CreatedAt
	return nil
}

func (f fakeDB) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	return f.exec(sql)
}

func (fakeDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errs.New(errs.CodeInternal, "fakeDB.Query")
}

func (f fakeDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return fakeRow{err: f.scan, row: f.row}
}

func TestIdempotencyStore_givesUpWhenTheRowKeepsVanishing(t *testing.T) {
	t.Parallel()
	conflict := func(string) (pgconn.CommandTag, error) { return pgconn.NewCommandTag("INSERT 0 0"), nil }
	s := db.NewIdempotencyStore(fakeDB{exec: conflict, scan: pgx.ErrNoRows}, testkit.NewClock(time.Time{}))
	if _, err := s.Begin(t.Context(), "user:u1", "k1", hashA()); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Begin = %v, want internal after the claim attempts run out", err)
	}
}

func TestIdempotencyStore_fakeFailuresAreClassified(t *testing.T) {
	t.Parallel()
	boom := &pgconn.PgError{Code: "08006"}
	s := db.NewIdempotencyStore(fakeDB{
		exec: func(string) (pgconn.CommandTag, error) { return pgconn.CommandTag{}, boom },
		scan: boom,
	}, testkit.NewClock(time.Time{}))
	if _, err := s.Begin(t.Context(), "user:u1", "k1", hashA()); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("Begin = %v, want db_unavailable", err)
	}
	failingSelect := db.NewIdempotencyStore(fakeDB{
		exec: func(string) (pgconn.CommandTag, error) { return pgconn.NewCommandTag("INSERT 0 0"), nil },
		scan: boom,
	}, testkit.NewClock(time.Time{}))
	if _, err := failingSelect.Begin(
		t.Context(),
		"user:u1",
		"k1",
		hashA(),
	); errs.CodeOf(
		err,
	) != errs.CodeDBUnavailable {
		t.Fatalf("Begin with a failing select = %v, want db_unavailable", err)
	}
}

func TestIdempotencyStore_takeoverThatLosesOrFailsIsHandled(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	stale := &sqlc.IdempotencyKey{
		ActorKey: "user:u1", Key: "k1", RequestHash: hashA(), Status: 1, CreatedAt: now.Add(-time.Hour),
	}
	takeover := func(sql string) bool { return strings.Contains(sql, "SET created_at") }
	lost := db.NewIdempotencyStore(fakeDB{row: stale, exec: func(sql string) (pgconn.CommandTag, error) {
		if takeover(sql) {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		}
		return pgconn.NewCommandTag("INSERT 0 0"), nil
	}}, testkit.NewClock(now))
	if _, err := lost.Begin(t.Context(), "user:u1", "k1", hashA()); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Begin when every takeover loses = %v, want internal after the attempts run out", err)
	}
	boom := &pgconn.PgError{Code: "08006"}
	failing := db.NewIdempotencyStore(fakeDB{row: stale, exec: func(sql string) (pgconn.CommandTag, error) {
		if takeover(sql) {
			return pgconn.CommandTag{}, boom
		}
		return pgconn.NewCommandTag("INSERT 0 0"), nil
	}}, testkit.NewClock(now))
	if _, err := failing.Begin(t.Context(), "user:u1", "k1", hashA()); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("Begin with a failing takeover = %v, want db_unavailable", err)
	}
}

func TestIdempotencyStore_racingTakeoversOwnTheAbandonedKeyExactlyOnce(t *testing.T) {
	t.Parallel()
	clk := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	s := db.NewIdempotencyStore(testkit.DB(t), clk)
	mustBegin(t, s, "user:u1", "k1", hashA())
	clk.Advance(6 * time.Minute)
	const n = 16
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
		t.Fatalf("owned = %d, in flight = %d, want 1 and %d: the created_at guard serializes the takeover", owned,
			inFlight, n-1)
	}
}
