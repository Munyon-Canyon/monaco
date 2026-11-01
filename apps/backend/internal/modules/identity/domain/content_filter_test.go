package domain

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func TestParseDisplayName_refusesAListedWord(t *testing.T) {
	t.Parallel()
	if _, err := ParseDisplayName("kys"); errs.CodeOf(err) != errs.CodeContentNotAllowed {
		t.Fatalf("ParseDisplayName(kys) = %v, want %s", err, errs.CodeContentNotAllowed)
	}
}

func TestParseBio_refusesAListedWord(t *testing.T) {
	t.Parallel()
	if _, err := ParseBio("trader, kys"); errs.CodeOf(err) != errs.CodeContentNotAllowed {
		t.Fatalf("ParseBio = %v, want %s", err, errs.CodeContentNotAllowed)
	}
}
