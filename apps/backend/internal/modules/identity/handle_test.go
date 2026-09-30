package identity_test

import (
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
)

func TestParseHandle_lowercasesAValidHandle(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]string{
		"KaiCenat":              "kaicenat",
		"abc":                   "abc",
		"a_1":                   "a_1",
		strings.Repeat("z", 20): strings.Repeat("z", 20),
	} {
		got, err := domain.ParseHandle(raw)
		if err != nil || got.String() != want {
			t.Errorf("ParseHandle(%q) = %q, %v, want %q", raw, got, err, want)
		}
	}
}

func TestParseHandle_refusesAnInvalidHandle(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		"", "ab", strings.Repeat("z", 21), "kai.cenat", "kai-cenat", "kai cenat", "kaï", "Kai", "@kai",
	} {
		if _, err := domain.ParseHandle(raw); errs.CodeOf(err) != errs.CodeHandleInvalid {
			t.Errorf("ParseHandle(%q) err = %v, want %s", raw, err, errs.CodeHandleInvalid)
		}
	}
}
