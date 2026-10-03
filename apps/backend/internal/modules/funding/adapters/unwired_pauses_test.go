package adapters_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func TestUnwiredPauses_FailsClosed(t *testing.T) {
	t.Parallel()
	pauses := adapters.UnwiredPauses{}
	_, isPausedErr := pauses.IsPaused(t.Context(), ids.CabalIDFrom(ids.Real{}.NewV7()))
	_, pausedCabalsErr := pauses.PausedCabals(t.Context())
	for name, err := range map[string]error{"IsPaused": isPausedErr, "PausedCabals": pausedCabalsErr} {
		if code := errs.CodeOf(err); errs.KindOf(code) != errs.KindUnavailable || !errs.Retryable(code) {
			t.Errorf("%s error code = %q, want a retryable KindUnavailable code", name, code)
		}
	}
}
