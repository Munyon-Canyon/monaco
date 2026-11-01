package trading_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

func TestRetryBridgeBack_aPriorActionThatFailedAfterItBroadcastIsNeverSentAgain(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.failedSale(t, false, multichain.ActionFailed)
	e.retryBack(t, swap)
	e.privy.failedSteps = []multichain.ActionStep{
		{Network: chain.NetworkSolana, TxHash: "solana-hash", State: multichain.StepConfirmed},
	}
	e.privy.script(multichain.ActionFailed)

	e.mustTick(t, 1, 1)

	if calls := e.privy.calls(); len(calls) != 1 {
		t.Fatalf("transfers = %+v, want only the first send", calls)
	}
	if status, action := e.bridgeBackOf(t, swap); status != "pending" || action != "" {
		t.Fatalf("bridge_back %s on %q, want pending with no second action", status, action)
	}
}
