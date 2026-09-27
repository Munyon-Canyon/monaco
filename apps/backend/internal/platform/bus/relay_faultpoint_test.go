//go:build faultpoints

package bus_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
)

func TestRelay_crashAfterPublishLeavesTheRowForTheRestartedRelay(t *testing.T) {
	t.Parallel()
	h := newRelayHarness(t)
	h.append(t, 1, "hi")
	crashed := func() (p any) {
		defer func() { p = recover() }()
		ctx, cancel := context.WithTimeout(faultpoint.Armed(h.ctx(t), faultpoint.AfterPublish), waitFor)
		defer cancel()
		h.relay.Run(ctx)
		return nil
	}()
	if crashed != (faultpoint.Crash{Name: faultpoint.AfterPublish}) {
		t.Fatalf("Run recovered %v, want Crash{after-publish}", crashed)
	}
	if n, left := msgs(t, h.bus), h.unpublished(t); n != 1 || left != 1 {
		t.Fatalf("after the crash the stream holds %d messages and %d rows are unpublished, want 1 and 1", n, left)
	}
	h.start(t)
	h.waitDrained(t)
	if n := msgs(t, h.bus); n != 1 {
		t.Fatalf("the restarted relay left %d messages in the stream, want 1 (deduplicated)", n)
	}
}
