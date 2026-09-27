//go:build faultpoints

package bus_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestRelay_crashAfterPublishConvergesToOneMessageAndOnePublishedRow(t *testing.T) {
	t.Parallel()
	h := newRelayHarness(t)
	h.append(t, 1, "hi")
	testkit.CrashAt(t, faultpoint.AfterPublish, func(ctx context.Context) error {
		h.relay.Once(ctx)
		return nil
	})
	var published int
	err := h.pool.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE published_at IS NOT NULL`).Scan(&published)
	if err != nil {
		t.Fatal(err)
	}
	if n, left := msgs(t, h.bus), h.unpublished(t); n != 1 || published != 1 || left != 0 {
		t.Fatalf("the stream holds %d messages, %d rows are published and %d unpublished, want 1, 1 and 0",
			n, published, left)
	}
}
