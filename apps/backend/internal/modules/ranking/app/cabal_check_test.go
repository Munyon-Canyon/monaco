package app_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type statusStub struct{ err error }

func (s statusStub) Status(context.Context, ids.CabalID) (cabalport.Status, error) {
	return cabalport.StatusBanned, s.err
}

func TestCheckCabal_passesTheStatusErrorThrough(t *testing.T) {
	t.Parallel()
	id := ids.CabalIDFrom(ids.Real{}.NewV7())
	if err := app.CheckCabal(statusStub{})(t.Context(), id); err != nil {
		t.Fatalf("known cabal: err = %v, want nil", err)
	}
	missing := statusStub{err: errs.New(errs.CodeCabalNotFound, "test")}
	if err := app.CheckCabal(missing)(t.Context(), id); errs.CodeOf(err) != errs.CodeCabalNotFound {
		t.Fatalf("unknown cabal: err = %v, want cabal_not_found", err)
	}
}
