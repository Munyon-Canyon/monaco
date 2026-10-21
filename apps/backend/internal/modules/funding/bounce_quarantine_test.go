package funding_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
)

func (f *bounceFixture) assertNotPaused(t *testing.T) {
	t.Helper()
	paused, err := adapters.NewPauses(f.pool).IsPaused(t.Context(), f.cabal.ID)
	if err != nil || paused.Paused {
		t.Fatalf("IsPaused = %+v, %v, want the cabal open for cash-outs, funding and trading", paused, err)
	}
	if externalPauses(t, f.pool) != 0 || countEvents(t, f.pool, events.TypeCabalPaused) != 0 {
		t.Fatal("an unsolicited deposit opened a cabal pause")
	}
}

func (f *bounceFixture) amount(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var amount string
	if err := f.pool.QueryRow(t.Context(), `SELECT amount::text FROM external_deposits WHERE id = $1`, id).
		Scan(&amount); err != nil {
		t.Fatal(err)
	}
	return amount
}

func TestQuarantine_ClosedRecipientAccountLeavesTheCabalOpen(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	f.chain.closed = true
	id := f.detected(t, flow08USDCSig)
	f.assertNotPaused(t)

	if err := f.bouncer.Start(bounceCtx(t), id); err != nil {
		t.Fatal(err)
	}

	f.assertNotPaused(t)
	if f.status(t, id) != "bounce_failed" || !f.resolved(t, id) || f.amount(t, id) != "25000000" {
		t.Fatalf("status = %s resolved = %t amount = %s, want a resolved bounce_failed row that keeps its amount",
			f.status(t, id), f.resolved(t, id), f.amount(t, id))
	}
	if len(f.transfers.builds) != 0 || f.ledgerRows(t) != 0 {
		t.Fatal("a failed refund signed a transfer or wrote ledger rows")
	}
}

func TestQuarantine_RejectedRefundLeavesTheCabalOpen(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	f.chain.failed = true
	id := f.detected(t, flow08USDCSig)

	if err := f.bouncer.Start(bounceCtx(t), id); err != nil {
		t.Fatal(err)
	}

	f.assertNotPaused(t)
	if f.status(t, id) != "bounce_failed" || !f.resolved(t, id) || f.ledgerRows(t) != 0 {
		t.Fatalf("status = %s resolved = %t, want a resolved bounce_failed row and no ledger rows",
			f.status(t, id), f.resolved(t, id))
	}
}

func TestQuarantine_RepeatedDepositsNeedNoPauseClearing(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	f.chain.closed = true
	first, second := f.detected(t, flow08USDCSig), f.detected(t, flow08StockSigListed(t, f))
	f.assertNotPaused(t)

	for _, id := range []uuid.UUID{first, second} {
		if err := f.bouncer.Start(bounceCtx(t), id); err != nil {
			t.Fatal(err)
		}
		f.assertNotPaused(t)
	}

	if !f.resolved(t, first) || !f.resolved(t, second) {
		t.Fatal("want both failed refunds resolved")
	}
	open, err := f.funding.ExternalDeposits().UnresolvedExternalDeposits(t.Context())
	if err != nil || len(open) != 2 {
		t.Fatalf("unresolved = %+v, %v, want both quarantined deposits still listed for ops", open, err)
	}
}

func TestQuarantine_GivingUpClearsAPauseOpenedBeforeThisChange(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	f.chain.closed = true
	id := f.detected(t, flow08USDCSig)
	exec(t, f.pool, `INSERT INTO cabal_pauses (id, cabal_id, reason, created_at, external_deposit_id)
		SELECT gen_random_uuid(), cabal_id, 'external_deposit', now(), id FROM external_deposits`)
	if externalPauses(t, f.pool) != 1 {
		t.Fatal("want the legacy pause open before the refund fails")
	}

	if err := f.bouncer.Start(bounceCtx(t), id); err != nil {
		t.Fatal(err)
	}

	if externalPauses(t, f.pool) != 0 || countEvents(t, f.pool, events.TypeCabalResumed) != 1 {
		t.Fatal("want the legacy pause resolved and one cabal.resumed")
	}
	paused, err := adapters.NewPauses(f.pool).IsPaused(t.Context(), f.cabal.ID)
	if err != nil || paused.Paused {
		t.Fatalf("IsPaused = %+v, %v, want not paused", paused, err)
	}
}

func TestQuarantine_AnOperatorRetryReopensTheRowUntilItSettles(t *testing.T) {
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
	f.chain.state = solana.StateProcessing

	if err := f.bouncer.Retry(bounceCtx(t), id, "ops"); err != nil {
		t.Fatal(err)
	}
	if f.status(t, id) != "bouncing" || f.resolved(t, id) {
		t.Fatalf("status = %s resolved = %t, want an open bouncing row", f.status(t, id), f.resolved(t, id))
	}

	f.chain.state = solana.StateFinalized
	if _, err := f.bouncer.Check(bounceCtx(t), id); err != nil {
		t.Fatal(err)
	}
	if f.status(t, id) != "returned" || !f.resolved(t, id) {
		t.Fatalf("status = %s resolved = %t, want a resolved returned row", f.status(t, id), f.resolved(t, id))
	}
	f.assertNotPaused(t)
}
