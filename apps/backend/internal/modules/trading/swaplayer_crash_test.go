//go:build faultpoints

package trading_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (e *layerEnv) crashAt(t *testing.T, point faultpoint.Name, wantAfterCrash string, wantSigned bool) {
	t.Helper()
	req := e.request(e.source())
	restarts := 0
	testkit.CrashAt(t, point, func(ctx context.Context) error {
		if restarts++; restarts == 2 {
			e.assertSurvivor(ctx, t, req.Source.ID, wantAfterCrash, wantSigned)
		}
		_, err := e.layer().Run(actorContext(ctx), req, nil)
		return err
	})
	if rows := e.swapIDs(t, req.Source.ID); len(rows) != 1 {
		t.Fatalf("the restarted run left %d swaps, want the one it found", len(rows))
	}
	e.assertAtMostOneTerminalEvent(t)
}

func (e *layerEnv) assertSurvivor(ctx context.Context, t *testing.T, source uuid.UUID, status string, signed bool) {
	t.Helper()
	var got string
	var stored, rows int
	if err := e.pool.QueryRow(ctx, `SELECT status, coalesce(octet_length(signed_tx), 0), count(*) OVER ()
		FROM swaps WHERE source_id = $1`, source).Scan(&got, &stored, &rows); err != nil {
		t.Fatalf("after the crash: %v", err)
	}
	if rows != 1 || got != status || (stored > 0) != signed {
		t.Fatalf("after the crash %d rows, the row is %s with %d signed bytes, want %s signed=%v",
			rows, got, stored, status, signed)
	}
}

func TestSwapLayer_CrashAfterCreate_LeavesACreatedRowNothingSigned(t *testing.T) {
	t.Parallel()
	newLayerEnv(t).crashAt(t, faultpoint.AfterCreate, "created", false)
}

func TestSwapLayer_CrashAfterSign_LeavesASubmittedRowWithTheSignedBytes(t *testing.T) {
	t.Parallel()
	newLayerEnv(t).crashAt(t, faultpoint.AfterSign, "submitted", true)
}

func TestSwapLayer_CrashAfterExecute_LeavesASubmittedRowTheSwapMayHaveLanded(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	e.jup.SetExecute("req-1", jupiter.ExecuteResult{Status: jupiter.StatusSuccess, OutAmount: 105_000_000})
	e.crashAt(t, faultpoint.AfterExecute, "submitted", true)
}

func TestSwapLayer_CrashBeforeCommitOfTheTerminalWrite_LeavesASubmittedRow(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	e.jup.SetExecute("req-1", jupiter.ExecuteResult{Status: jupiter.StatusSuccess, OutAmount: 105_000_000})
	req := e.request(e.source())
	armed := faultpoint.ArmedAfter(actorContext(t.Context()), faultpoint.BeforeCommit, 2)
	crashed := func() (p any) {
		defer func() { p = recover() }()
		_, _ = e.layer().Run(armed, req, nil)
		return nil
	}()
	if crashed != (faultpoint.Crash{Name: faultpoint.BeforeCommit}) {
		t.Fatalf("the run ended with %v, want a crash before the terminal commit", crashed)
	}
	e.assertSurvivor(t.Context(), t, req.Source.ID, "submitted", true)
	if n := e.terminalEvents(t, e.swapIDs(t, req.Source.ID)[0]); n != 0 {
		t.Fatalf("%d terminal events survived a rolled back transaction", n)
	}
	got, err := e.run(t, req)
	if err != nil || got.Status != domain.StatusSubmitted {
		t.Fatalf("the restarted run = %+v, %v, want the submitted row", got, err)
	}
}
