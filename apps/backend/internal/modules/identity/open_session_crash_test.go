//go:build faultpoints

package identity_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestOpenSession_aCrashBeforeCommitLeavesNothingAndTheRetryCreatesOneAccount(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	f.privy.Seed(alicePhone())
	runs := 0
	var me app.Me
	testkit.CrashAt(t, faultpoint.BeforeCommit, func(ctx context.Context) error {
		runs++
		if got := f.tally(ctx, t); runs == 2 && got != (tally{PrivyCreates: 1}) {
			t.Fatalf("after the crash: %+v, want nothing written but the wallet Privy already made", got)
		}
		var err error
		me, err = f.handler.Handle(ctx, app.OpenSession{Token: string(alice)})
		return err
	})
	if got := f.tally(t.Context(), t); got != (tally{1, 1, 1, 1, 1}) || me.ID.IsZero() {
		t.Fatalf("after the retry: %+v, want one user, wallet, user.created, hint and Privy create", got)
	}
}
