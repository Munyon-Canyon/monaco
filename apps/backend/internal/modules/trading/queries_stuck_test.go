package trading_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type stuckDB struct {
	portDB
	clock *testkit.Clock
}

func newStuckDB(t *testing.T) stuckDB {
	t.Helper()
	d := newSwapDB(t)
	clk := testkit.NewClock(d.now.Add(10 * time.Minute))
	port := trading.New(module.Deps{Pool: d.pool, Clock: clk}).Queries()
	return stuckDB{portDB: portDB{swapDB: d, port: port}, clock: clk}
}

func (d stuckDB) created(t *testing.T, age time.Duration) sqlc.InsertCreatedParams {
	t.Helper()
	row := d.swapDB.created(d.ids.NewV7(), usdcMint)
	row.CreatedAt = d.clock.Now().Add(-age)
	d.insert(t, row)
	return row
}

func (d stuckDB) submitted(t *testing.T, submittedAge time.Duration, sig string) sqlc.InsertCreatedParams {
	t.Helper()
	row := d.created(t, time.Hour)
	_, err := d.q.MarkSubmitted(t.Context(), sqlc.MarkSubmittedParams{
		ID: row.ID, ExecuteRequestID: "req-" + sig, SignedTx: []byte{1}, TxSignature: sig,
		SubmittedAt: d.clock.Now().Add(-submittedAge),
	})
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func swapIDs(views []trading.SwapView) []ids.SwapID {
	out := make([]ids.SwapID, len(views))
	for i, v := range views {
		out[i] = v.ID
	}
	return out
}

func TestQueries_stuckListsUnfinishedSwapsPastTheCutoffOldestFirst(t *testing.T) {
	t.Parallel()
	d := newStuckDB(t)
	staleCreated := d.created(t, 9*time.Minute)
	d.created(t, time.Minute)
	staleSubmitted := d.submitted(t, 7*time.Minute, "sig-stale")
	d.submitted(t, time.Minute, "sig-fresh")
	done := d.submitted(t, 8*time.Minute, "sig-done")
	d.confirm(t, done.ID)
	failed := d.created(t, time.Hour)
	d.fail(t, failed.ID, string(domain.FailureNeverSubmitted))
	got, err := d.port.Stuck(t.Context(), 5*time.Minute, 10)
	wantIDs := []ids.SwapID{ids.SwapIDFrom(staleCreated.ID), ids.SwapIDFrom(staleSubmitted.ID)}
	if err != nil || !slices.Equal(swapIDs(got), wantIDs) {
		t.Fatalf("Stuck = %+v, %v", got, err)
	}
	if got[1].Status != domain.StatusSubmitted || got[1].TxSignature != "sig-stale" {
		t.Fatalf("submitted row = %+v", got[1])
	}
	limited, err := d.port.Stuck(t.Context(), 5*time.Minute, 1)
	if err != nil || len(limited) != 1 || limited[0].ID != got[0].ID {
		t.Fatalf("Stuck limit 1 = %+v, %v", limited, err)
	}
	count, err := d.port.CountStuck(t.Context(), 5*time.Minute)
	if err != nil || count != 2 {
		t.Fatalf("CountStuck = %d, %v", count, err)
	}
	if count, err = d.port.CountStuck(t.Context(), 8*time.Minute); err != nil || count != 1 {
		t.Fatalf("CountStuck over eight minutes = %d, %v", count, err)
	}
}

func TestQueries_stuckDefaultsToTheSystemClock(t *testing.T) {
	t.Parallel()
	d := newPortDB(t)
	row := d.created(d.ids.NewV7(), usdcMint)
	d.insert(t, row)
	got, err := trading.New(module.Deps{Pool: d.pool}).Queries().Stuck(t.Context(), time.Minute, 5)
	if err != nil || len(got) != 1 || got[0].ID != ids.SwapIDFrom(row.ID) {
		t.Fatalf("Stuck = %+v, %v", got, err)
	}
}

func TestQueries_stuckRefusesARowMissingItsSignature(t *testing.T) {
	t.Parallel()
	d := newStuckDB(t)
	row := d.created(t, time.Hour)
	const corrupt = `UPDATE swaps SET status = 'submitted', submitted_at = $2 WHERE id = $1`
	if _, err := d.pool.Exec(t.Context(), corrupt, row.ID, d.now); err != nil {
		t.Fatal(err)
	}
	if _, err := d.port.Stuck(t.Context(), time.Minute, 5); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("Stuck err = %v, want decode_failed", err)
	}
}

func TestQueries_executeRequestID(t *testing.T) {
	t.Parallel()
	d := newStuckDB(t)
	submitted := d.submitted(t, time.Minute, "sig-1")
	got, err := d.port.ExecuteRequestID(t.Context(), ids.SwapIDFrom(submitted.ID))
	if err != nil || got != "req-sig-1" {
		t.Fatalf("ExecuteRequestID = %q, %v", got, err)
	}
	created := d.created(t, time.Minute)
	if got, err = d.port.ExecuteRequestID(t.Context(), ids.SwapIDFrom(created.ID)); err != nil || got != "" {
		t.Fatalf("ExecuteRequestID of an unsubmitted swap = %q, %v", got, err)
	}
	if _, err = d.port.ExecuteRequestID(t.Context(), ids.SwapIDFrom(d.ids.NewV7())); errs.CodeOf(err) !=
		errs.CodeSwapNotFound {
		t.Fatalf("ExecuteRequestID of an unknown swap err = %v, want swap_not_found", err)
	}
}

func TestQueries_stuckReadsReportInternalWhenTheDatabaseIsDown(t *testing.T) {
	t.Parallel()
	d := newStuckDB(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.port.Stuck(ctx, time.Minute, 1); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("Stuck err = %v", err)
	}
	if _, err := d.port.CountStuck(ctx, time.Minute); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("CountStuck err = %v", err)
	}
	if _, err := d.port.ExecuteRequestID(ctx, ids.SwapIDFrom(d.ids.NewV7())); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("ExecuteRequestID err = %v", err)
	}
}

func TestQueries_stuckQueryCount(t *testing.T) {
	t.Parallel()
	d := newStuckDB(t)
	row := d.submitted(t, 7*time.Minute, "sig-1")
	check := func(name string, call func() error) {
		t.Helper()
		testkit.AssertQueries(t, name, func() {
			if err := call(); err != nil {
				t.Fatal(err)
			}
		})
	}
	check("trading Stuck", func() error { _, err := d.port.Stuck(t.Context(), 5*time.Minute, 50); return err })
	check("trading CountStuck", func() error { _, err := d.port.CountStuck(t.Context(), 5*time.Minute); return err })
	check("trading ExecuteRequestID", func() error {
		_, err := d.port.ExecuteRequestID(t.Context(), ids.SwapIDFrom(row.ID))
		return err
	})
}
