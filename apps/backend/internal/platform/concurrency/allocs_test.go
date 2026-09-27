package concurrency_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/concurrency"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestAllocsFanOut(t *testing.T) {
	in := items(8)
	testkit.AssertAllocs(t, "concurrency.FanOut 8 items limit 4", func() {
		if _, err := concurrency.FanOut(t.Context(), 4, in, double); err != nil {
			t.Fatal(err)
		}
	})
}
