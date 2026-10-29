package referrals_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/referrals"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestCodes_resolvesAKnownCodeToItsOwner(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	id := seedOwner(t, pool, owner{code: "k7m4qx2p"})

	got, ok, err := referrals.New(module.Deps{Pool: pool}).Codes().Resolve(t.Context(), "k7m4qx2p")

	if err != nil || !ok || got != id {
		t.Fatalf("Resolve = %v, %v, %v; want %v, true, nil", got, ok, err, id)
	}
}

func TestCodes_reportsAnUnknownCodeAsNotFound(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)

	got, ok, err := referrals.New(module.Deps{Pool: pool}).Codes().Resolve(t.Context(), "kaicenot")

	if err != nil || ok || got != (ids.UserID{}) {
		t.Fatalf("Resolve = %v, %v, %v; want zero, false, nil", got, ok, err)
	}
}

func TestCodes_returnsAReadFailure(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	got, ok, err := referrals.New(module.Deps{Pool: pool}).Codes().Resolve(ctx, "kaicenat")

	if err == nil || ok || got != (ids.UserID{}) {
		t.Fatalf("Resolve = %v, %v, %v; want zero, false, an error", got, ok, err)
	}
}
