package flows_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func TestSteps_Flow30StepsHaveAScriptOnlyOnceReady(t *testing.T) {
	t.Parallel()
	var got []string
	for _, s := range flows.Steps() {
		if s.Flow != "30" {
			continue
		}
		ready := s.Status == flows.StepReady && s.Script != nil
		pending := s.Status == flows.StepPending && s.Script == nil
		if s.Owner == "" || !ready && !pending {
			t.Errorf("step %d %q = %+v, want an owner and a script exactly when ready", s.Order, s.Name, s)
		}
		got = append(got, s.Name)
	}
	want := []string{
		"EVM deposit credited", "Fund cabal", "Routed buy on EVM", "Routed sell", "Governed routed sell",
		"EVM withdrawal",
	}
	if len(got) != len(want) {
		t.Fatalf("flow 30 steps = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("flow 30 steps = %q, want %q", got, want)
		}
	}
}
