//go:build faultpoints

package funding_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func crashThenReturn(t *testing.T, point faultpoint.Name) *bounceFixture {
	t.Helper()
	f := newBounceFixture(t)
	detect := f.funding.DetectExternalDeposit()
	testkit.CrashAt(t, point, func(ctx context.Context) error {
		ctx = observability.WithActor(ctx, "system:poller.funding.treasury-reconcile")
		if _, err := detect.Handle(ctx, app.DetectExternalDeposit{
			Signature: flow08USDCSig, CabalID: f.cabal.ID, Treasury: flow08Treasury, Source: domain.SourceReconcile,
		}); err != nil {
			return err
		}
		var id uuid.UUID
		if err := f.pool.QueryRow(ctx, `SELECT id FROM external_deposits WHERE signature = $1`,
			string(flow08USDCSig)).Scan(&id); err != nil {
			return err
		}
		return f.bouncer.Start(ctx, id)
	})
	f.clock.Advance(app.BounceSweepAge + time.Second)
	f.tick(t)

	if row := f.only(t, flow08USDCSig); row.status != "returned" {
		t.Fatalf("row = %+v, want returned", row)
	}
	if externalPauses(t, f.pool) != 0 || countEvents(t, f.pool, events.TypeCabalPaused) != 1 ||
		countEvents(t, f.pool, events.TypeCabalExternalDepositDetected) != 1 ||
		countEvents(t, f.pool, events.TypeCabalExternalDepositBounced) != 1 ||
		countEvents(t, f.pool, events.TypeCabalResumed) != 1 {
		t.Fatal("after the crash and restart, want one pause opened and ended and one of each event")
	}
	if f.ledgerRows(t) != 0 {
		t.Fatal("the bounce wrote ledger rows")
	}
	return f
}

func TestFlow08_DetectExternalDeposit_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	f := crashThenReturn(t, faultpoint.BeforeCommit)
	f.requireOneTransferBack(t)
}

func TestFlow08_DetectExternalDeposit_CrashAfterSign(t *testing.T) {
	t.Parallel()
	f := crashThenReturn(t, faultpoint.AfterSign)
	if len(f.transfers.builds) != 2 || len(f.transfers.sent) != 1 {
		t.Fatalf("builds = %d, sends = %d, want the unstored signature dropped and one send",
			len(f.transfers.builds), len(f.transfers.sent))
	}
}

func TestFlow08_DetectExternalDeposit_CrashAfterBroadcast(t *testing.T) {
	t.Parallel()
	f := crashThenReturn(t, faultpoint.AfterBroadcast)
	f.requireOneTransferBack(t)
}
