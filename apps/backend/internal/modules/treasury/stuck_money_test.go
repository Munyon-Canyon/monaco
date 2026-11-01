package treasury_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func seedFundTransfers(t *testing.T, pool *pgxpool.Pool, cabal, user uuid.UUID, at time.Time) (stuck uuid.UUID) {
	t.Helper()
	gen := testkit.NewIDs(41)
	stuck = gen.NewV7()
	for _, row := range []struct {
		id       uuid.UUID
		status   string
		failCode any
	}{{stuck, "landed", nil}, {gen.NewV7(), "failed", "not_sent"}} {
		if _, err := pool.Exec(t.Context(), `INSERT INTO fund_transfers (id, user_id, cabal_id, amount_micros,
			from_address, to_address, status, fail_code, signed_tx, tx_signature, last_valid_block_height, submitted_at,
			landed_at, created_at)
			VALUES ($1, $2, $3, 5000000, 'a', 'b', $4, $5, '\x00', 'sig-' || $4::text, 1, $6, $6, $6)`,
			row.id, user, cabal, row.status, row.failCode, at); err != nil {
			t.Fatal(err)
		}
	}
	return stuck
}

func TestStuckMoney_ListsOnlyTheNonFinalTreasuryRows(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	cabal := testkit.NewCabal(t, pool)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{Handle: "dana"}).ID.UUID()
	clk := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	old := clk.Now().Add(-time.Hour)
	stuck := seedFundTransfers(t, pool, cabal.ID.UUID(), user, old)
	reader := app.StuckMoney{Reads: pool, Clock: clk}
	rows, err := reader.List(t.Context(), "", 5*time.Minute, 10)
	if err != nil || len(rows) != 1 || rows[0].Kind != money.StuckFundTransfer || rows[0].Ref != stuck.String() ||
		rows[0].Status != "landed" || rows[0].UserID != user || rows[0].CabalID != cabal.ID.UUID() ||
		!rows[0].Since.Equal(old) {
		t.Fatalf("List = %+v, %v, want the landed transfer only", rows, err)
	}
	for _, miss := range []struct {
		kind string
		age  time.Duration
	}{{money.StuckWindDown, 5 * time.Minute}, {"", 2 * time.Hour}} {
		if rows, err := reader.List(t.Context(), miss.kind, miss.age, 10); err != nil || len(rows) != 0 {
			t.Fatalf("List(%q, %v) = %+v, %v, want none", miss.kind, miss.age, rows, err)
		}
	}
}

func TestStuckMoney_ReportsAClosedDatabase(t *testing.T) {
	t.Parallel()
	open := testkit.DB(t)
	closed, err := pgxpool.NewWithConfig(t.Context(), open.Config())
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	reader := app.StuckMoney{Reads: closed, Clock: testkit.NewClock(time.Unix(0, 0))}
	if _, err := reader.List(t.Context(), "", time.Minute, 1); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("List = %v, want db_unavailable", err)
	}
}
