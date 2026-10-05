package funding_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestBounceSignatures_OwnsOnlyRecordedBounces(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	cabal := testkit.NewCabal(t, pool)
	if _, err := pool.Exec(t.Context(), `INSERT INTO external_deposits
		(id, signature, cabal_id, sender, mint, amount, source, status, bounce_signature, detected_at)
		VALUES (gen_random_uuid(), 'inbound', $1, 'sender', 'mint', 5, 'webhook', 'bouncing', 'bounce', $2)`,
		cabal.ID.UUID(), clock.Real{}.Now()); err != nil {
		t.Fatal(err)
	}
	owner := adapters.NewBounceSignatures(pool)
	for sig, want := range map[string]bool{"bounce": true, "inbound": false, "other": false} {
		got, err := owner.OwnsSignature(t.Context(), chain.Signature(sig))
		if err != nil || got != want {
			t.Errorf("OwnsSignature(%s) = %t, %v, want %t", sig, got, err, want)
		}
	}
	exec(t, pool, `ALTER TABLE external_deposits RENAME TO external_deposits_gone`)
	if _, err := owner.OwnsSignature(t.Context(), "bounce"); err == nil {
		t.Fatal("OwnsSignature on a missing table = nil error")
	}
}
