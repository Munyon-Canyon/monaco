package app

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
)

func TestCashOutPayouts_moveRefusesATransitionTheTableLacks(t *testing.T) {
	t.Parallel()
	job := payoutJob{status: domain.CashOutCompleted}
	if err := (&CashOutPayouts{}).move(t.Context(), nil, job, domain.CashOutFail, ""); errs.CodeOf(err) !=
		errs.CodeVersionConflict {
		t.Fatalf("move = %v, want version_conflict", err)
	}
}

func TestCashOutSweeper_aStuckRowWithUnreadableAmountsIsRefused(t *testing.T) {
	t.Parallel()
	row := sqlc.CashOutSalesStuckRow{ShareUnits: "x", PayoutMicros: "y", SellUsdcMicros: "z"}
	if _, err := cashOutStartedFor(row); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("cashOutStartedFor = %v, want decode_failed", err)
	}
}
