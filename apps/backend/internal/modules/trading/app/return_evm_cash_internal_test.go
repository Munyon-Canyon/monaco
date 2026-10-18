package app

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func TestReturnable_refusesALandedAmountItCannotParse(t *testing.T) {
	t.Parallel()
	row := sqlc.ReturnableCashRow{BridgeOut: "succeeded", Network: "base", Status: "failed", LandedMicros: "-1"}

	if _, err := returnable(ids.SwapID{}, row); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("returnable err = %v, want internal", err)
	}
}
