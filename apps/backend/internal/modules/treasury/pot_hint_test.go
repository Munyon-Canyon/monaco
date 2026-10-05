package treasury_test

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
)

func TestPotHint_CashOut(t *testing.T) {
	t.Parallel()
	t.Run("completed", func(t *testing.T) {
		t.Parallel()
		r := newPayoutRig(t)
		r.seed(t, 1, domain.PayoutBroadcast)
		r.chain.set(seededSig(1), landed())
		r.mustAdvance(t)
		r.mustAdvance(t)
		r.wantJob(t, "completed", "")
		if !slices.Contains(r.hints.keys, events.CabalActivityChangedHint(r.cabal)) {
			t.Fatalf("hints = %v, want %s", r.hints.keys, events.CabalActivityChangedHint(r.cabal))
		}
	})
	t.Run("failed", func(t *testing.T) {
		t.Parallel()
		r := newPayoutRig(t)
		r.seed(t, 1, domain.PayoutBroadcast)
		r.chain.set(seededSig(1), refused())
		r.mustAdvance(t)
		r.wantJob(t, "failed", string(errs.CodePayoutFailed))
		if !slices.Contains(r.hints.keys, events.CabalActivityChangedHint(r.cabal)) {
			t.Fatalf("hints = %v, want %s", r.hints.keys, events.CabalActivityChangedHint(r.cabal))
		}
	})
	t.Run("rolled back", func(t *testing.T) {
		t.Parallel()
		r := newPayoutRig(t)
		r.seed(t, 1, domain.PayoutBroadcast)
		r.chain.set(seededSig(1), landed())
		if _, err := r.f.pool.Exec(t.Context(), refuse("events", "NEW.type = 'cashout.completed'")); err != nil {
			t.Fatal(err)
		}
		if err := r.advance(t, 0); err == nil {
			t.Fatal("Advance = nil, want the completion rolled back")
		}
		r.wantJob(t, "paying", "")
		if len(r.hints.keys) != 0 {
			t.Fatalf("hints = %v, want none after a rollback", r.hints.keys)
		}
	})
}
