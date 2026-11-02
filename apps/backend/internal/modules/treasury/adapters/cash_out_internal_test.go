package adapters

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestCashOutConfirmed_refusesAnOutAmountThatIsNotUSDCMicros(t *testing.T) {
	t.Parallel()
	over, err := money.ParseUnits("18446744073709551616")
	if err != nil {
		t.Fatal(err)
	}
	e := events.TradeConfirmed{Source: events.TradeSource{Kind: cashOutSource}, OutAmount: over}
	err = CashOut{}.Confirmed(t.Context(), db.Tx{}, e, time.Time{})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Confirmed = %v, want invalid_input", err)
	}
}
