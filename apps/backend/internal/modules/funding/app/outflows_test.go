package app_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func TestNoFundTransfers(t *testing.T) {
	t.Parallel()
	got, err := (app.NoFundTransfers{}).InFlightMicros(t.Context(), ids.UserIDFrom(ids.Real{}.NewV7()))
	if err != nil || !got.IsZero() {
		t.Fatalf("InFlightMicros = %v, %v", got, err)
	}
}
