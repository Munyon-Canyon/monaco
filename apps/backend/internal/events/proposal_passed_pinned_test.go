package events_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

func TestProposalPassedPinnedNetworkRoundTripsAndIsOmittedWhenEmpty(t *testing.T) {
	t.Parallel()
	pinned := routedPassedFixture(t)
	pinned.Network = chain.NetworkBase
	raw, err := json.Marshal(pinned)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"network":"base"`)) {
		t.Fatalf("pinned payload lacks the network: %s", raw)
	}
	ev, err := events.Decode(events.TypeProposalPassed, 1, raw)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := ev.(events.ProposalPassed); !ok || got.Network != chain.NetworkBase {
		t.Fatalf("decoded = %+v, want network base", ev)
	}
	unpinned, err := json.Marshal(routedPassedFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(unpinned, &top); err != nil {
		t.Fatal(err)
	}
	if _, ok := top["network"]; ok {
		t.Fatalf("unpinned payload carries a network: %s", unpinned)
	}
}
