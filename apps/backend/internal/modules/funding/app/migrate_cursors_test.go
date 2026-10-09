package app

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestCursorMigratorReportsReadAndWriteFailures(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	m := NewCursorMigrator(pool, db.New(pool, testkit.NewIDs(95), clock.Real{}), clock.Real{}, testkit.USDCMint)
	if _, err := m.migrate(
		t.Context(),
		[]sqlc.DepositCursorsToMigrateRow{{WalletAddress: "bad"}},
	); errs.CodeOf(
		err,
	) != errs.CodeInvalidAddress {
		t.Fatalf("migrate with an invalid wallet = %v, want %s", err, errs.CodeInvalidAddress)
	}
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	rows := []sqlc.DepositCursorsToMigrateRow{{WalletAddress: string(user.Address), UserID: user.ID.UUID()}}
	for _, table := range []string{"deposit_watch_accounts", "deposit_watch_wallets"} {
		if _, err := pool.Exec(t.Context(), `DROP TABLE `+table+` CASCADE`); err != nil {
			t.Fatal(err)
		}
		if _, err := m.migrate(t.Context(), rows); err == nil {
			t.Fatalf("migrate without %s error = nil", table)
		}
	}
}

func TestCursorMigratorReportsAnUnreadableCursorTable(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	pool.Close()
	m := NewCursorMigrator(pool, db.New(pool, testkit.NewIDs(96), clock.Real{}), clock.Real{}, testkit.USDCMint)
	if _, err := m.Run(t.Context()); err == nil {
		t.Fatal("Run error = nil")
	}
}

func TestCursorMigratorPagesThroughTheCursors(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	m := NewCursorMigrator(pool, db.New(pool, testkit.NewIDs(97), clock.Real{}), clock.Real{}, testkit.USDCMint)
	m.page = 1
	for range 3 {
		user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
		if _, err := pool.Exec(t.Context(),
			`INSERT INTO deposit_cursors (wallet_address, last_signature, cursor_slot, scanned_at)
			VALUES ($1, '', 0, now())`, user.Address); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := m.Run(t.Context()); err != nil || n != 3 {
		t.Fatalf("Run = %d, %v; want 3 wallets over three pages", n, err)
	}
	var high *string
	if err := pool.QueryRow(t.Context(), `SELECT max(high_signature) FROM deposit_watch_accounts`).
		Scan(&high); err != nil ||
		high != nil {
		t.Fatalf("high signature = %v, %v; want NULL for a cursor without a signature", high, err)
	}
}
