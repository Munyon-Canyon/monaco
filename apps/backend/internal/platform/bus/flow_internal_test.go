package bus

import "testing"

func TestFlow_associatesOnlyFlowHandlers(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]string{
		"trading.cashout_sell": "14", "treasury.cashout": "14", "treasury.cashout_payout": "14",
		"treasury.cashout.confirmed": "14", "treasury.cashout.failed": "14", "treasury.cashout.blocked": "14",
		"trading.engine": "11", "trading.engine.retry": "12", "treasury.trades": "",
		"funding.bounce": "08", "notify.notify_test_requested": "24",
	} {
		if got := flow(name); got != want {
			t.Errorf("flow(%q) = %q, want %q", name, got, want)
		}
	}
}
