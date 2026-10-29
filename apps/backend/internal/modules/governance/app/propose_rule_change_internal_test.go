package app

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func TestInsertFailed_anyOtherFailureIsInternal(t *testing.T) {
	t.Parallel()
	if got := insertFailed(errs.New(errs.CodeDBUnavailable, "db"), "op"); errs.CodeOf(got) != errs.CodeInternal {
		t.Fatalf("insertFailed = %v, want internal", got)
	}
}
