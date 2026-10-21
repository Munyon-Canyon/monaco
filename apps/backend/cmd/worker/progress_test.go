package main

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestDBProgress_reportsTheOldestUnresolvedMoneyIntentAndIsZeroWhenNothingIsOpen(t *testing.T) {
	t.Parallel()
	pool, b := testkit.DB(t), testkit.NATS(t)
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	reader := dbProgress{pool: pool, bus: b.Conn, clock: testkit.NewClock(now)}

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
