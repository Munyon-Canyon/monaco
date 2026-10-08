package ranking_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
)

func TestTriggers_insertOneTriggerPerMoneyEvent(t *testing.T) {
	t.Parallel()
	cabal := uuid.NewSHA1(uuid.Nil, []byte("cabal"))
	for name, tc := range map[string]struct {
		handler string
		ev      events.Event
		reason  string
	}{
		"trade":    {"ranking.triggers", events.TradeConfirmed{V: 1, CabalID: cabal}, "trade_confirmed"},
		"funded":   {"ranking.triggers.funded", events.Funded{V: 1, CabalID: cabal}, "cabal_funded"},
		"cash out": {"ranking.triggers.cashed_out", events.CashOutCompleted{V: 1, CabalID: cabal}, "cashout_completed"},
		"cash out started": {
			"ranking.triggers.cash_out_started", events.CashOutStarted{V: 1, CabalID: cabal}, "cashout_started",
		},
		"cash out partial": {
			"ranking.triggers.cash_out_partial", events.CashOutPartial{V: 1, CabalID: cabal}, "cashout_partial",
		},
		"cash out failed": {
			"ranking.triggers.cash_out_failed", events.CashOutFailed{V: 1, CabalID: cabal}, "cashout_failed",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newDeliverer(t)
			id := d.event(t, tc.ev)
			d.clock.Advance(time.Minute)
			if duplicate, err := d.deliverAs(t.Context(), t, id, tc.handler, tc.ev); err != nil || duplicate {
				t.Fatalf("first delivery = duplicate %t, %v; want it to succeed", duplicate, err)
			}
			if duplicate, err := d.deliverAs(t.Context(), t, id, tc.handler, tc.ev); err != nil || !duplicate {
				t.Fatalf("redelivery = duplicate %t, %v; want a duplicate", duplicate, err)
			}
			want := cabal.String() + " " + tc.reason + " true"
			if got := d.triggers(t); len(got) != 1 || got[0] != want {
				t.Fatalf("triggers = %q, want one %q stamped at the delivery time", got, want)
			}
		})
	}
}
