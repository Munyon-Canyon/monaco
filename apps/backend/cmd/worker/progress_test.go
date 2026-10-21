package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestDBProgress_reportsTheOldestUnresolvedMoneyIntentAndIsZeroWhenNothingIsOpen(t *testing.T) {
	t.Parallel()
	pool, b := testkit.DB(t), testkit.NATS(t)
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	reader := newDBProgress(pool, b.Conn, testkit.NewClock(now))

	got, err := reader.Progress(t.Context())
	if err != nil || got != (progress{}) {
		t.Fatalf("Progress with nothing open = %+v, %v; want zero", got, err)
	}

	insert := func(sig, status string, seen time.Time) {
		t.Helper()
		_, err := pool.Exec(t.Context(),
			`INSERT INTO deposit_candidates (tx_signature, wallet_address, user_id, slot, source, status, seen_at)
			 VALUES ($1, 'wallet', $2, 1, 'poller', $3, $4)`, sig, uuid.New(), status, seen)
		if err != nil {
			t.Fatal(err)
		}
	}
	insert("old-credited", "credited", now.Add(-time.Hour))
	insert("newer-pending", "pending", now.Add(-30*time.Second))
	insert("older-pending", "pending", now.Add(-5*time.Minute))

	got, err = reader.Progress(t.Context())
	if err != nil || got.MoneyIntentOldest != 5*time.Minute {
		t.Fatalf("Progress = %+v, %v; want the oldest pending intent at 5m0s and the credited one ignored", got, err)
	}
}

func TestDBProgress_failsWhenAnyOfItsThreeReadsFails(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeUpstreamUnavailable, "test.read")
	ok := dbProgress{
		clock:     testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)),
		intent:    func(context.Context) (time.Time, error) { return time.Time{}, nil },
		outbox:    func(context.Context) (db.Backlog, error) { return db.Backlog{}, nil },
		consumers: func(context.Context, time.Time) (bus.ConsumerBacklog, error) { return bus.ConsumerBacklog{}, nil },
	}
	if _, err := ok.Progress(t.Context()); err != nil {
		t.Fatalf("Progress with every read passing = %v, want nil", err)
	}
	intent, outbox, consumers := ok, ok, ok
	intent.intent = func(context.Context) (time.Time, error) { return time.Time{}, boom }
	outbox.outbox = func(context.Context) (db.Backlog, error) { return db.Backlog{}, boom }
	consumers.consumers = func(context.Context, time.Time) (bus.ConsumerBacklog, error) {
		return bus.ConsumerBacklog{}, boom
	}
	for name, p := range map[string]dbProgress{"intent": intent, "outbox": outbox, "consumers": consumers} {
		if _, err := p.Progress(t.Context()); !errors.Is(err, boom) {
			t.Errorf("Progress with failing %s = %v, want the read's error", name, err)
		}
	}
}

func TestNewDBProgress_failsWhenTheIntentQueryCannotRun(t *testing.T) {
	t.Parallel()
	closed, err := pgxpool.NewWithConfig(t.Context(), testkit.DB(t).Config())
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	reader := newDBProgress(closed, testkit.NATS(t).Conn, testkit.NewClock(now))
	if _, err := reader.Progress(t.Context()); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Progress on a closed pool = %v, want internal", err)
	}
}
