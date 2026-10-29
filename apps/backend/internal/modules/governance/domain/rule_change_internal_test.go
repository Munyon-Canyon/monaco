package domain

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func TestRuleChange_marshalReportsAValueJSONCannotCarry(t *testing.T) {
	t.Parallel()
	change := RuleChange{rule: RuleShowSides, from: make(chan int), to: true}
	if _, err := change.MarshalJSON(); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("MarshalJSON err = %v, want internal", err)
	}
}
