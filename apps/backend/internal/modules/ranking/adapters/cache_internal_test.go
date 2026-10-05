package adapters

import (
	"testing"
)

func TestAwait_returnsTheFinishedCallsResult(t *testing.T) {
	t.Parallel()
	flight := &call[int]{done: make(chan struct{}), val: 3}
	close(flight.done)
	if got, err := await(t.Context(), flight); err != nil || *got != 3 {
		t.Fatalf("await = %v, %v, want 3", got, err)
	}
}
