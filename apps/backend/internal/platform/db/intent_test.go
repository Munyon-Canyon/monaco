package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func intentKey(key string, hash []byte) db.IntentKey {
	return db.IntentKey{ActorKey: "user:u1", Key: key, Operation: "test.op", RequestHash: hash}
}

func bindIn(
	t *testing.T, u *db.UnitOfWork, k db.IntentKey, id uuid.UUID, now time.Time,
) (got uuid.UUID, replayed bool, err error) {
	t.Helper()
	err = u.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		var err error
		got, replayed, err = k.Bind(ctx, tx.Queries(), id, now)
		return err
	})
	return got, replayed, err
}

func TestIntentKey_bindsOnceThenReplaysTheFirstResource(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	u := db.New(testkit.DB(t), testkit.NewIDs(5), testkit.NewClock(now))
	first, second := uuid.New(), uuid.New()

	got, replayed, err := bindIn(t, u, intentKey("k1", hashA()), first, now)
	if err != nil || replayed || got != first {
		t.Fatalf("first Bind = (%v, %v, %v), want (%v, false, nil)", got, replayed, err, first)
	}
	got, replayed, err = bindIn(t, u, intentKey("k1", hashA()), second, now)
	if err != nil || !replayed || got != first {
		t.Fatalf("second Bind = (%v, %v, %v), want the first resource %v replayed", got, replayed, err, first)
	}
	got, replayed, err = bindIn(t, u, intentKey("k2", hashA()), second, now)
	if err != nil || replayed || got != second {
		t.Fatalf("another key = (%v, %v, %v), want a fresh binding", got, replayed, err)
	}
}

func TestIntentKey_refusesAnotherRequestAndAnotherOperationUnderTheSameKey(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	u := db.New(testkit.DB(t), testkit.NewIDs(5), testkit.NewClock(now))
	if _, _, err := bindIn(t, u, intentKey("k1", hashA()), uuid.New(), now); err != nil {
		t.Fatal(err)
	}
	_, _, err := bindIn(t, u, intentKey("k1", hashB()), uuid.New(), now)
	if errs.CodeOf(err) != errs.CodeIdempotencyMismatch {
		t.Fatalf("another hash = %v, want %s", err, errs.CodeIdempotencyMismatch)
	}
	other := intentKey("k1", hashA())
	other.Operation = "test.other"
	if _, _, err = bindIn(t, u, other, uuid.New(), now); errs.CodeOf(err) != errs.CodeIdempotencyMismatch {
		t.Fatalf("another operation = %v, want %s", err, errs.CodeIdempotencyMismatch)
	}
}

func TestIntentKey_anUnboundKeyIsNeverStored(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	pool := testkit.DB(t)
	u := db.New(pool, testkit.NewIDs(5), testkit.NewClock(now))
	id := uuid.New()
	got, replayed, err := bindIn(t, u, db.IntentKey{}, id, now)
	if err != nil || replayed || got != id {
		t.Fatalf("Bind without a key = (%v, %v, %v), want the resource back unbound", got, replayed, err)
	}
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM money_intents`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("money_intents = %d (%v), want 0", n, err)
	}
}

func TestUnbindIntent_freesTheKeyForANewResource(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	u := db.New(testkit.DB(t), testkit.NewIDs(5), testkit.NewClock(now))
	first, second := uuid.New(), uuid.New()
	if _, _, err := bindIn(t, u, intentKey("k1", hashA()), first, now); err != nil {
		t.Fatal(err)
	}
	if err := u.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		return db.UnbindIntent(ctx, tx.Queries(), "test.op", first)
	}); err != nil {
		t.Fatal(err)
	}
	got, replayed, err := bindIn(t, u, intentKey("k1", hashA()), second, now)
	if err != nil || replayed || got != second {
		t.Fatalf("Bind after Unbind = (%v, %v, %v), want a fresh binding %v", got, replayed, err, second)
	}
}

func conflictTag() (pgconn.CommandTag, error) { return pgconn.NewCommandTag("INSERT 0 0"), nil }

func TestIntentKey_failuresAreCodedErrors(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	boom := &pgconn.PgError{Code: "08006"}
	tests := []struct {
		name string
		fake intentFake
		want errs.Code
	}{
		{"the insert fails", intentFake{
			exec: func() (pgconn.CommandTag, error) { return pgconn.CommandTag{}, boom },
		}, errs.CodeDBUnavailable},
		{"the read fails", intentFake{
			exec: conflictTag, row: func(...any) error { return boom },
		}, errs.CodeDBUnavailable},
		{"the row keeps vanishing", intentFake{
			exec: conflictTag, row: func(...any) error { return pgx.ErrNoRows },
		}, errs.CodeInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := intentKey("k1", hashA()).Bind(t.Context(), tt.fake, uuid.New(), now)
			if errs.CodeOf(err) != tt.want {
				t.Fatalf("Bind = %v, want %s", err, tt.want)
			}
		})
	}
}

func TestUnbindIntent_aFailureIsACodedError(t *testing.T) {
	t.Parallel()
	fake := intentFake{exec: func() (pgconn.CommandTag, error) {
		return pgconn.CommandTag{}, &pgconn.PgError{Code: "08006"}
	}}
	if err := db.UnbindIntent(t.Context(), fake, "test.op", uuid.New()); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("UnbindIntent = %v, want %s", err, errs.CodeDBUnavailable)
	}
}
