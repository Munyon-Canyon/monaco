package adapters

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/treasuryapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestGetHoldingPerformance_refusesACallerWithoutASession(t *testing.T) {
	t.Parallel()
	_, err := HTTP{}.GetHoldingPerformance(t.Context(), api.GetHoldingPerformanceRequestObject{Symbol: "AAPLx"})
	if errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("GetHoldingPerformance without an actor = %v, want unauthorized", err)
	}
}

func TestPnlPoints_mapsEachPointAndNeverReturnsNil(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	got := pnlPoints([]app.PnLPoint{{At: at, PnLMicros: money.SignedMicrosFromInt64(-5)}})
	if len(got) != 1 || !got[0].At.Equal(at) || got[0].PnlMicros != -5 {
		t.Fatalf("pnlPoints = %+v, want one point of -5", got)
	}
	if empty := pnlPoints(nil); empty == nil || len(empty) != 0 {
		t.Fatalf("pnlPoints(nil) = %#v, want an empty non-nil slice", empty)
	}
}
