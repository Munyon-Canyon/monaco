package app

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type failingSearchReader struct{}

func (failingSearchReader) SearchUsers(
	context.Context, sqlc.SearchUsersParams,
) ([]sqlc.SearchUsersRow, error) {
	return nil, errs.New(errs.CodeInternal, "test.SearchUsers")
}

func TestSearchUsers_wrapsReadFailure(t *testing.T) {
	t.Parallel()
	_, err := searchUsers(t.Context(), failingSearchReader{}, ids.UserID{}, "maya")
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("code = %s, want %s", errs.CodeOf(err), errs.CodeInternal)
	}
}
