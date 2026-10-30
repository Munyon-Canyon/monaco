package adapters

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/treasuryapi"
)

func TestGetHoldingPerformance_refusesACallerWithoutASession(t *testing.T) {
	t.Parallel()
	_, err := HTTP{}.GetHoldingPerformance(t.Context(), api.GetHoldingPerformanceRequestObject{Symbol: "AAPLx"})
	if errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("GetHoldingPerformance without an actor = %v, want unauthorized", err)
	}
}
