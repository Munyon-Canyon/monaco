package app

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func TestAvailabilityReasonMapsTheHandleCodes(t *testing.T) {
	t.Parallel()
	for code, want := range map[string]string{
		string(errs.CodeHandleTaken):    "taken",
		string(errs.CodeHandleReserved): "reserved",
		string(errs.CodeHandleTooSoon):  "too_soon",
		string(errs.CodeInternal):       "invalid",
	} {
		if got := availabilityReason(code); got != want {
			t.Errorf("availabilityReason(%s) = %q, want %q", code, got, want)
		}
	}
}
