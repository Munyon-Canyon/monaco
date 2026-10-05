//go:build faultpoints

package funding_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestFlow08_DetectExternalDeposit_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	f := newFlow08(t)
	detect := f.funding.DetectExternalDeposit()
	testkit.CrashAt(t, faultpoint.BeforeCommit, func(ctx context.Context) error {
		ctx = observability.WithActor(ctx, "system:poller.funding.treasury-reconcile")
		_, err := detect.Handle(ctx, app.DetectExternalDeposit{
			Signature: flow08USDCSig, CabalID: f.cabal.ID, Treasury: flow08Treasury, Source: domain.SourceReconcile,
		})
		return err
	})
	if f.only(t, flow08USDCSig).status != "detected" || externalPauses(t, f.pool) != 1 ||
		countEvents(t, f.pool, events.TypeCabalPaused) != 1 ||
		countEvents(t, f.pool, events.TypeCabalExternalDepositDetected) != 1 {
		t.Fatal("after the crash and retry, want one detected row, one pause and one of each event")
	}
}
