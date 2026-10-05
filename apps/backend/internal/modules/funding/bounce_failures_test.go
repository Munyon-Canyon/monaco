package funding_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestBounce_DependencyFailuresBeforeSigningLeaveTheRowDetected(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeRPCUnavailable, "test")
	for name, arrange := range map[string]func(*bounceFixture){
		"mint":      func(f *bounceFixture) { f.chain.mintErr = down },
		"accounts":  func(f *bounceFixture) { f.chain.accountsErr = down },
		"treasury":  func(f *bounceFixture) { f.deps.Treasuries = bounceTreasury{err: down} },
		"transfers": func(f *bounceFixture) { f.deps.Transfers = func() (app.Transfers, error) { return nil, down } },
		"build":     func(f *bounceFixture) { f.transfers.buildErr = errs.New(errs.CodePrivyUnavailable, "test") },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newBounceFixture(t)
			arrange(f)
			id := f.detected(t, flow08USDCSig)

			err := app.NewBouncer(f.deps).Start(bounceCtx(t), id)

			if !errs.Retryable(errs.CodeOf(err)) || f.status(t, id) != "detected" {
				t.Fatalf("Start = %v with status %s, want a retryable error and detected", err, f.status(t, id))
			}
		})
	}
}

func TestBounce_RefusedSigningFails(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	f.transfers.buildErr = errs.New(errs.CodeInvalidInput, "test")
	id := f.detected(t, flow08USDCSig)

	if err := f.bouncer.Start(bounceCtx(t), id); err != nil || f.status(t, id) != "bounce_failed" {
		t.Fatalf("Start = %v with status %s, want bounce_failed", err, f.status(t, id))
	}
}

func TestBounce_InvalidRecipientFails(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	id := f.detected(t, flow08USDCSig)
	exec(t, f.pool, `UPDATE external_deposits SET return_address = 'not-an-address'`)

	if err := f.bouncer.Start(bounceCtx(t), id); err != nil || f.status(t, id) != "bounce_failed" {
		t.Fatalf("Start = %v with status %s, want bounce_failed", err, f.status(t, id))
	}
}

func TestBounce_SignedAfterAnotherWriterMovedTheRowStoresNothing(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	id := f.detected(t, flow08USDCSig)
	f.transfers.onBuild = func() { exec(t, f.pool, `UPDATE external_deposits SET status = 'held'`) }

	if err := f.bouncer.Start(bounceCtx(t), id); err != nil {
		t.Fatal(err)
	}
	if f.status(t, id) != "held" || len(f.transfers.sent) != 0 {
		t.Fatalf("status = %s with %d sends, want held and nothing sent", f.status(t, id), len(f.transfers.sent))
	}
}

func TestBounce_BroadcastFailureNaksWithTheSignatureStored(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	f.transfers.sendErr = errs.New(errs.CodeRPCUnavailable, "test")
	id := f.detected(t, flow08USDCSig)

	err := f.bouncer.Start(bounceCtx(t), id)

	if !errs.Retryable(errs.CodeOf(err)) || f.status(t, id) != "bouncing" {
		t.Fatalf("Start = %v with status %s, want a retryable error and bouncing", err, f.status(t, id))
	}
}

func TestBounce_ConfirmAfterAnotherWriterReSignedResolvesNothing(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	id := f.detected(t, flow08USDCSig)
	f.chain.onStatus = func() { exec(t, f.pool, `UPDATE external_deposits SET bounce_signature = 'other'`) }

	if err := f.bouncer.Start(bounceCtx(t), id); err != nil {
		t.Fatal(err)
	}
	if f.status(t, id) != "bouncing" || countEvents(t, f.pool, events.TypeCabalExternalDepositBounced) != 0 {
		t.Fatal("a confirm of a replaced signature returned the deposit")
	}
}

func TestBounce_CheckIgnoresARowThatIsNotBouncing(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	id := f.detected(t, flow08USDCSig)

	done, err := f.bouncer.Check(bounceCtx(t), id)

	if !done || err != nil || f.chain.queries != 0 {
		t.Fatalf("Check = %v, %v after %d queries, want done with no chain read", done, err, f.chain.queries)
	}
}

func TestBounce_CancelledWhileWaitingIsRetryable(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	f.chain.state = solana.StateProcessing
	f.deps.Clock = f.clock
	id := f.detected(t, flow08USDCSig)
	ctx, cancel := context.WithCancel(bounceCtx(t))
	f.chain.onStatus = cancel

	err := app.NewBouncer(f.deps).Start(ctx, id)

	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("Start = %v, want upstream_unavailable", err)
	}
}

func TestBounceConsumer_RecordsTheDeliveryOnlyOnSuccess(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	id := f.detected(t, flow08USDCSig)
	uow := db.New(f.pool, testkit.NewIDs(82), f.clock)
	ev := events.CabalExternalDepositDetected{V: 1, ExternalDepositID: id, CabalID: f.cabal.ID.UUID()}
	consumer := adapters.Bounce{Bouncer: f.bouncer, UoW: uow}
	failing := adapters.Bounce{Bouncer: app.NewBouncer(app.BounceDeps{Reads: f.pool}), UoW: uow}
	missing := ev
	missing.ExternalDepositID = ids.Real{}.NewV7()
	var eventID uuid.UUID
	if err := f.pool.QueryRow(t.Context(), `SELECT id FROM events WHERE type = $1`,
		events.TypeCabalExternalDepositDetected).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	delivery := bus.Delivery{Handler: "funding.bounce", EventID: ids.EventIDFrom(eventID), At: f.clock.Now()}

	failed, err := failing.Handle(t.Context(), delivery, missing), consumer.Handle(t.Context(), delivery, ev)

	if failed == nil || err != nil || f.status(t, id) != "returned" {
		t.Fatalf("Handle = %v then %v, want an error, then the deposit returned", failed, err)
	}
	var deliveries int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM event_deliveries
		WHERE handler = 'funding.bounce'`).Scan(&deliveries); err != nil || deliveries != 1 {
		t.Fatalf("deliveries = %d, %v, want one", deliveries, err)
	}
}
