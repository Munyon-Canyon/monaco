package adapters_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func TestUnwiredBalances_FailsClosed(t *testing.T) {
	t.Parallel()
	_, err := (adapters.UnwiredBalances{}).Available(t.Context(), ids.UserIDFrom(ids.Real{}.NewV7()))
	if got := errs.CodeOf(err); got != errs.CodeRPCUnavailable {
		t.Fatalf("Available error code = %q, want %q", got, errs.CodeRPCUnavailable)
	}
}
