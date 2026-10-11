package flows_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func TestSteps_Flow30StartsWithEveryStepPending(t *testing.T) {
	t.Parallel()
	var got []string
	for _, s := range flows.Steps() {
		if s.Flow != "30" {
			continue
		}
		if s.Status != flows.StepPending || s.Owner == "" || s.Script != nil {
			t.Errorf("step %d %q = %+v, want pending with an owner and no script yet", s.Order, s.Name, s)
		}
		got = append(got, s.Name)
	}
	want := []string{"EVM deposit credited", "Fund cabal", "Routed buy on EVM", "Routed sell", "EVM withdrawal"}
	if len(got) != len(want) {
		t.Fatalf("flow 30 steps = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("flow 30 steps = %q, want %q", got, want)
		}
	}
}
