package funding_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type candidateFixture struct {
	pool *pgxpool.Pool
	uow  *db.UnitOfWork
	ids  *testkit.IDs
	now  time.Time
	user testkit.SeededUser
}

func newCandidateFixture(t *testing.T) candidateFixture {
	t.Helper()
	pool := testkit.DB(t)
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	return candidateFixture{
		pool: pool, uow: db.New(pool, testkit.NewIDs(300), testkit.NewClock(now)), ids: testkit.NewIDs(301), now: now,
		user: testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true}),
	}
}

func (f candidateFixture) candidate() app.DepositCandidate {
	return app.DepositCandidate{
		Signature: chain.Signature(depositSignature), Wallet: f.user.Address, UserID: f.user.ID,
		Slot: 42, BlockTime: f.now.Add(-time.Minute), Source: "poller",
	}
}

func (f candidateFixture) recorder() app.CandidateRecorder {
	return app.NewCandidateRecorder(f.ids, func() time.Time { return f.now })
}

func (f candidateFixture) record(t *testing.T, candidates ...app.DepositCandidate) int {
	t.Helper()
	var inserted int
	ctx := observability.WithActor(t.Context(), "system:funding.deposits")
	err := f.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		inserted, err = f.recorder().Record(ctx, tx, candidates)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return inserted
}

func (f candidateFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCandidateRecorder_insertsPendingRowAndSeenEvent(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	if got := f.record(t, f.candidate()); got != 1 {
		t.Fatalf("Record = %d, want 1", got)
	}
	var status, source string
	var slot int64
	if err := f.pool.QueryRow(
		t.Context(), `SELECT status, source, slot FROM deposit_candidates WHERE tx_signature = $1`, depositSignature,
	).Scan(&status, &source, &slot); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || source != "poller" || slot != 42 {
		t.Fatalf("row = %s %s %d, want pending poller 42", status, source, slot)
	}
	if n := f.count(t, `SELECT count(*) FROM events WHERE type = $1`, events.TypeDepositCandidateSeen); n != 1 {
		t.Fatalf("candidate_seen events = %d, want 1", n)
	}
}

func TestCandidateRecorder_duplicateAppendsNothing(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	f.record(t, f.candidate())
	again := f.candidate()
	again.Source = "recovery"
	if got := f.record(t, again); got != 0 {
		t.Fatalf("duplicate Record = %d, want 0", got)
	}
	if n := f.count(t, `SELECT count(*) FROM deposit_candidates`); n != 1 {
		t.Fatalf("candidates = %d, want 1", n)
	}
	if n := f.count(t, `SELECT count(*) FROM events WHERE type = $1`, events.TypeDepositCandidateSeen); n != 1 {
		t.Fatalf("candidate_seen events = %d, want 1", n)
	}
}

func TestCandidateRecorder_eventsOnlyForInsertedRows(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	f.record(t, f.candidate())
	fresh := f.candidate()
	fresh.Signature = "second-signature"
	fresh.BlockTime = time.Time{}
	if got := f.record(t, f.candidate(), fresh); got != 1 {
		t.Fatalf("mixed Record = %d, want 1", got)
	}
	if n := f.count(t, `SELECT count(*) FROM events WHERE type = $1`, events.TypeDepositCandidateSeen); n != 2 {
		t.Fatalf("candidate_seen events = %d, want 2", n)
	}
	if n := f.count(t, `SELECT count(*) FROM deposit_candidates WHERE block_time IS NULL`); n != 1 {
		t.Fatalf("null block_time rows = %d, want 1", n)
	}
}

func TestCandidateRecorder_failuresRollBack(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	missingActor := f.candidate()
	if err := f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		_, err := f.recorder().Record(ctx, tx, []app.DepositCandidate{missingActor})
		return err
	}); err == nil {
		t.Fatal("Record without actor error = nil")
	}
	if n := f.count(t, `SELECT count(*) FROM deposit_candidates`); n != 0 {
		t.Fatalf("candidates after failed append = %d, want 0", n)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := f.uow.Do(t.Context(), func(_ context.Context, tx db.Tx) error {
		_, err := f.recorder().Record(canceled, tx, []app.DepositCandidate{f.candidate()})
		return err
	}); err == nil {
		t.Fatal("Record with cancelled context error = nil")
	}
}
