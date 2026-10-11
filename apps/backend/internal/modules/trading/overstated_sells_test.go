package trading_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestOverstatedSells_aClosedDatabaseReportsNothing(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	pool.Close()
	report, err := app.OverstatedSells(t.Context(), pool, newChainFills(), usdcToken())
	if err == nil || len(report.Sells) != 0 || len(report.Unread) != 0 {
		t.Fatalf("OverstatedSells = %+v, %v; want an error and no rows", report, err)
	}
}
