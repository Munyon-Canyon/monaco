package governance_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
)

func TestParseChoice(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"yes", "no"} {
		if got, err := domain.ParseChoice(raw); err != nil || string(got) != raw {
			t.Errorf("ParseChoice(%q) = %q, %v", raw, got, err)
		}
	}
	for _, raw := range []string{"", "YES", "abstain"} {
		if _, err := domain.ParseChoice(raw); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("ParseChoice(%q) err = %v, want invalid_input", raw, err)
		}
	}
}
