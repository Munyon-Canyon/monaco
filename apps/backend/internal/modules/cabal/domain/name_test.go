package domain_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
)

func TestParseName_refusesAListedWord(t *testing.T) {
	t.Parallel()
	if _, err := domain.ParseName("kys club"); errs.CodeOf(err) != errs.CodeContentNotAllowed {
		t.Fatalf("ParseName = %v, want %s", err, errs.CodeContentNotAllowed)
	}
	if got, err := domain.ParseName("Alpha Cabal"); err != nil || got.String() != "Alpha Cabal" {
		t.Fatalf("ParseName(Alpha Cabal) = %q, %v, want it accepted", got.String(), err)
	}
}
