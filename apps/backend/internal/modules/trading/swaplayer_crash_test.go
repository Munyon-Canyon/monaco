//go:build faultpoints

package trading_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

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
