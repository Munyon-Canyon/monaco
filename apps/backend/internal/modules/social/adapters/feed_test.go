package adapters

import "testing"

func TestFeedName_usesHandleWithoutADisplayName(t *testing.T) {
	t.Parallel()
	if got := feedName("", "alice"); got != "alice" {
		t.Fatalf("name = %q, want alice", got)
	}
}
