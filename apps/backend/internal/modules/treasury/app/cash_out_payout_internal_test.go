package app

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
)

func TestCashOutPayouts_moveRefusesATransitionTheTableLacks(t *testing.T) {
	t.Parallel()
	job := payoutJob{status: domain.CashOutCompleted}
	if err := (&CashOutPayouts{}).move(t.Context(), nil, job, domain.CashOutFail, ""); errs.CodeOf(err) !=
		errs.CodeVersionConflict {
		t.Fatalf("move = %v, want version_conflict", err)
	}
}
