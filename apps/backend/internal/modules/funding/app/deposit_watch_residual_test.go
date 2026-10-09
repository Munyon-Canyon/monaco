package app

import (
	"context"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type dropLedger struct {
	pool  sqlc.DBTX
	table string
}

func (l dropLedger) WalletLedgerMicros(
	ctx context.Context, _ ids.UserID, _ chain.SolanaAddress,
) (money.SignedMicros, int, error) {
	_, err := l.pool.Exec(ctx, `DROP TABLE `+l.table+` CASCADE`)
	return money.SignedMicros{}, 0, err
}

func TestDepositWatchReconcileFailsWithoutWalletRows(t *testing.T) {
	t.Parallel()
	p, ctx, _ := watchWithDroppedTable(t, "deposit_watch_wallets")
	if _, _, err := p.reconcile(ctx, newWatchBudget(ctx, 1, time.Second)); err == nil {
		t.Fatal("reconcile error = nil")
	}
}

func TestDepositWatchReconcileFailsWhenTheUpdateCannotCommit(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	if _, err := pool.Exec(
		t.Context(),
		`INSERT INTO deposit_watch_wallets (
			wallet_address, user_id, first_seen_slot, first_seen_at, opening_micros, reconcile_due_at)
		VALUES ($1, $2, 0, now(), 0, now() - interval '1 minute')`, user.Address, user.ID.UUID(),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(
		t.Context(),
		`INSERT INTO deposit_watch_accounts (
			token_account, wallet_address, canonical, state, observed_slot, recovery_due_at)
		VALUES ('account', $1, true, 'open', 1, now())`, user.Address,
	); err != nil {
		t.Fatal(err)
	}
	p := testWatch(pool, db.New(pool, testkit.NewIDs(92), clock.Real{}), &watchRPC{}, nil)
	p.ledger = dropLedger{pool: pool, table: "deposit_watch_wallets"}
	ctx := observability.WithActor(t.Context(), "system:test")
	if _, _, err := p.reconcile(ctx, newWatchBudget(ctx, 1, time.Second)); err == nil {
		t.Fatal("reconcile error = nil")
	}
}

func assertGaugeReadFails(t *testing.T, table string) {
	t.Helper()
	pool := testkit.DB(t)
	if _, err := pool.Exec(t.Context(), `DROP TABLE `+table+` CASCADE`); err != nil {
		t.Fatal(err)
	}
	reader := sdkmetric.NewManualReader()
	ObserveDepositWatch(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test"), pool, clock.Real{})
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err == nil {
		t.Fatalf("collect without %s error = nil", table)
	}
}

func TestObserveDepositWatchReportsResidualGaugeReadFailures(t *testing.T) {
	t.Parallel()
	assertGaugeReadFails(t, "deposit_watch_wallets")
}

func TestObserveDepositWatchReportsPendingGaugeReadFailures(t *testing.T) {
	t.Parallel()
	assertGaugeReadFails(t, "deposit_candidates")
}
