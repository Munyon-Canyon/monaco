package funding_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestStuckMoney_ReportsAClosedDatabase(t *testing.T) {
	t.Parallel()
	reader := app.StuckMoney{Reads: closedPool(t, testkit.DB(t)), Clock: testkit.NewClock(time.Unix(0, 0))}
	if _, err := reader.List(t.Context(), "", time.Minute, 1); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("List = %v, want db_unavailable", err)
	}
	if _, err := reader.Summary(t.Context(), time.Minute); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("Summary = %v, want db_unavailable", err)
	}
}

func TestBounceQueue_ReportsAClosedDatabaseFromTheStatusList(t *testing.T) {
	t.Parallel()
	_, err := app.BounceQueue(t.Context(), closedPool(t, testkit.DB(t)), app.OpenBounceStatuses(), 1)
	if errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("BounceQueue = %v, want db_unavailable", err)
	}
}
