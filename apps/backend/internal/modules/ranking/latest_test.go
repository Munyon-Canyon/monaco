package ranking_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestQueries_answersNotFoundBeforeTheFirstRun(t *testing.T) {
	t.Parallel()
	d := newDeliverer(t)
	queries := ranking.New(module.Deps{Pool: d.pool}).Queries()
	_, err := queries.LatestRun(t.Context())
	if errs.CodeOf(err) != errs.CodeNotFound {
		t.Fatalf("LatestRun() = %v, want not found", err)
	}
}

func seedRuns(t *testing.T, d deliverer, now time.Time) {
	t.Helper()
	q := sqlc.New(d.pool)
	for _, finished := range []time.Time{now.Add(-time.Hour), now} {
		run := sqlc.InsertLeaderboardRunParams{
			RunID: d.gen.NewV7(), AsOf: finished, PricesAsOf: finished, StartedAt: finished.Add(-time.Second),
			FinishedAt: finished, RowsWritten: 4, CabalsExcluded: 1,
		}
		if err := q.InsertLeaderboardRun(t.Context(), run); err != nil {
			t.Fatal(err)
		}
	}
	cabal := uuid.NewSHA1(uuid.Nil, []byte("cabal"))
	for i, value := range []int64{5, 9} {
		snapshot := sqlc.InsertCabalValueSnapshotParams{
			CabalID: cabal, At: now.Add(time.Duration(i) * time.Minute), ValueMicros: value,
			NavPerShareMicros: 2, TotalShares: 3,
		}
		if err := q.InsertCabalValueSnapshot(t.Context(), snapshot); err != nil {
			t.Fatal(err)
		}
	}
}

func TestQueries_returnTheLatestRunAndEachCabalsLatestValue(t *testing.T) {
	t.Parallel()
	d := newDeliverer(t)
	now := d.clock.Now()
	seedRuns(t, d, now)
	queries := ranking.New(module.Deps{Pool: d.pool}).Queries()
	run, err := queries.LatestRun(t.Context())
	if err != nil || !run.FinishedAt.Equal(now) || run.RowsWritten != 4 || run.CabalsExcluded != 1 {
		t.Fatalf("LatestRun() = %+v, %v, want the newest run", run, err)
	}
	values, err := queries.LatestCabalValues(t.Context())
	if err != nil || len(values) != 1 || values[0].Value != money.MicrosFromUint64(9) ||
		values[0].NavPerShare != money.MicrosFromUint64(2) || values[0].TotalShares != money.SharesUnitsFromUint64(3) {
		t.Fatalf("LatestCabalValues() = %+v, %v, want the newest snapshot of the one cabal", values, err)
	}
}

func TestQueries_refusesANegativeStoredAmount(t *testing.T) {
	t.Parallel()
	d := newDeliverer(t)
	snapshot := sqlc.InsertCabalValueSnapshotParams{CabalID: d.gen.NewV7(), At: d.clock.Now(), ValueMicros: -1}
	if err := sqlc.New(d.pool).InsertCabalValueSnapshot(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	queries := ranking.New(module.Deps{Pool: d.pool}).Queries()
	_, err := queries.LatestCabalValues(t.Context())
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("LatestCabalValues() = %v, want decode failed", err)
	}
}
