package trading_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
)

func TestFinishFailed_theSQLGuardAgreesWithTheStateMachineForEveryFailureCode(t *testing.T) {
	t.Parallel()
	d := newSwapDB(t)
	for _, from := range []domain.Status{domain.StatusCreated, domain.StatusSubmitted} {
		for _, code := range domain.FailureCodes() {
			row := d.created(d.ids.NewV7(), usdcMint)
			d.insert(t, row)
			if from == domain.StatusSubmitted && d.submit(t, row.ID, row.ID.String(), "sig-"+row.ID.String()) != 1 {
				t.Fatalf("submit %s", row.ID)
			}
			_, err := domain.Next(from, domain.Fail(code))
			want := int64(0)
			if err == nil {
				want = 1
			}
			if n := d.fail(t, row.ID, string(code)); n != want {
				t.Errorf("FinishFailed(%s) from %s changed %d rows, want %d as domain.Next says", code, from, n, want)
			}
		}
	}
}
