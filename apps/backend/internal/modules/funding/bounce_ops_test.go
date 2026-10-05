package funding_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
)

func TestBounceOps_HoldEndsThePauseWithoutABounce(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	id := f.detected(t, flow08USDCSig)

	if err := f.bouncer.Hold(bounceCtx(t), id, "ops"); err != nil {
		t.Fatal(err)
	}
	if f.status(t, id) != "held" || externalPauses(t, f.pool) != 0 ||
		countEvents(t, f.pool, events.TypeCabalResumed) != 1 ||
		countEvents(t, f.pool, events.TypeCabalExternalDepositBounced) != 0 || f.ledgerRows(t) != 0 {
		t.Fatal("want held, the pause ended, cabal.resumed, no bounce event and no ledger rows")
	}
	if err := f.bouncer.Start(bounceCtx(t), id); err != nil || len(f.transfers.builds) != 0 {
		t.Fatalf("Start on a held deposit = %v with %d builds, want nothing sent", err, len(f.transfers.builds))
	}
}

func TestBounceOps_RetrySendsANewBounceForAFailedOne(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	f.chain.closed = true
	id := f.detected(t, flow08USDCSig)
	if err := f.bouncer.Start(bounceCtx(t), id); err != nil {
		t.Fatal(err)
	}
	if err := f.bouncer.SetReturnAddress(bounceCtx(t), id, string(withdrawTo), "ops"); err != nil {
		t.Fatal(err)
	}
	f.chain.closed = false

	if err := f.bouncer.Retry(bounceCtx(t), id, "ops"); err != nil {
		t.Fatal(err)
	}
	if f.status(t, id) != "returned" || f.transfers.builds[0].To != withdrawTo || externalPauses(t, f.pool) != 0 {
		t.Fatalf("status = %s, want returned to the new address with the pause ended", f.status(t, id))
	}
}

func TestBounceOps_RefuseRowsInTheWrongStatus(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	id := f.detected(t, flow08USDCSig)

	if err := f.bouncer.Retry(bounceCtx(t), id, "ops"); errs.CodeOf(err) != errs.CodeVersionConflict {
		t.Fatalf("Retry on detected = %v, want a version conflict", err)
	}
	if err := f.bouncer.Start(bounceCtx(t), id); err != nil {
		t.Fatal(err)
	}
	for name, err := range map[string]error{
		"hold":               f.bouncer.Hold(bounceCtx(t), id, "ops"),
		"retry":              f.bouncer.Retry(bounceCtx(t), id, "ops"),
		"set-return-address": f.bouncer.SetReturnAddress(bounceCtx(t), id, string(withdrawTo), "ops"),
	} {
		if errs.CodeOf(err) != errs.CodeVersionConflict {
			t.Errorf("%s on returned = %v, want a version conflict", name, err)
		}
	}
	if f.status(t, id) != "returned" {
		t.Fatalf("status = %s, want returned untouched", f.status(t, id))
	}
}

func TestBounceOps_SetReturnAddressValidatesTheAddress(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	id := f.detected(t, flow08USDCSig)

	err := f.bouncer.SetReturnAddress(bounceCtx(t), id, "not-an-address", "ops")

	if errs.CodeOf(err) != errs.CodeInvalidAddress {
		t.Fatalf("SetReturnAddress = %v, want invalid_address", err)
	}
}

func TestBounceOps_MissingRowsAreErrors(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	missing := f.cabal.ID.UUID()

	if f.bouncer.Hold(bounceCtx(t), missing, "ops") == nil || f.bouncer.Retry(bounceCtx(t), missing, "ops") == nil {
		t.Fatal("hold or retry of a missing deposit succeeded")
	}
}

func TestUnresolvedExternalDeposits_ListsOpenRowsForAdmins(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	f.chain.closed = true
	id := f.detected(t, flow08USDCSig)
	if err := f.deliver(t, flow08DustSig); err != nil {
		t.Fatal(err)
	}
	if err := f.bouncer.Start(bounceCtx(t), id); err != nil {
		t.Fatal(err)
	}

	rows, err := f.funding.ExternalDeposits().UnresolvedExternalDeposits(t.Context())

	if err != nil || len(rows) != 1 || rows[0].ID != id || rows[0].Status != domain.ExternalBounceFailed ||
		rows[0].Sender != flow08Sender || rows[0].Amount != "25000000" {
		t.Fatalf("Unresolved = %+v, %v, want the bounce_failed row only", rows, err)
	}
	if _, err := f.bouncer.UnresolvedExternalDeposits(cancelled(t)); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Unresolved on a cancelled context = %v, want internal", err)
	}
}

func cancelled(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	return ctx
}
