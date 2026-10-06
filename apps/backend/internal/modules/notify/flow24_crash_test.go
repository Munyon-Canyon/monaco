//go:build faultpoints

package notify_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestFlow24_Notify_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	user := r.user(t, "active")
	r.device(t, user, token('a'))
	d, e := r.trigger(t, opsActor, user)

	testkit.CrashAt(t, faultpoint.BeforeCommit, func(ctx context.Context) error {
		return r.handleIn(ctx, r.sender, d, e, app.Test{})
	})

	r.wantStates(t, d, map[ids.UserID]string{user: "delivered"})
	r.wantSentEvents(t, d, 1)
	r.wantRecorded(t, d, 1)
	if sent := r.sender.Sent(); len(sent) != 1 {
		t.Fatalf("sends = %d, want the one after the crash: nothing went out before the commit", len(sent))
	}
}
