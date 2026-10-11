package trading_test

import (
	"context"
	"sync"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

const solanaTreasury = chain.SolanaAddress("9ixcyg5nNxGCJLtSyJYibP7EgQBw4BfpNLbDe7GQ14eh")

type topUp struct {
	wallet          chain.SolanaAddress
	transfersBefore int
}

type topUpFake struct {
	mu    sync.Mutex
	privy *privyFake
	err   error
	calls []topUp
}

func (f *topUpFake) TopUpSOL(_ context.Context, wallet chain.SolanaAddress) error {
	before := len(f.privy.calls())
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, topUp{wallet: wallet, transfersBefore: before})
	return f.err
}

func (f *topUpFake) list() []topUp {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]topUp(nil), f.calls...)
}

func TestRouteExec_topsUpTheCabalsSolanaTreasuryBeforeTheBridgeOut(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}

	e.mustBuy(t, e.request(), domain.StatusConfirmed)

	if got := e.topUps.list(); len(got) != 1 || got[0] != (topUp{wallet: e.wallet.Address}) {
		t.Fatalf("top-ups = %+v, want one for the cabal's Solana treasury before any transfer", got)
	}
}

func TestBridge_aTopUpFailureLeavesTheStepUnsent(t *testing.T) {
	t.Parallel()
	e := newBridgeEnv(t)
	e.topUps.err = errs.New(errs.CodeUpstreamUnavailable, "relayer.TopUps.TopUpSOL")

	if _, err := e.run(t.Context()); errs.CodeOf(err) != errs.CodeUpstreamUnavailable || len(e.privy.calls()) != 0 {
		t.Fatalf("Run err = %v after %d transfers, want the top-up error and no transfer", err, len(e.privy.calls()))
	}
	if s := e.step(t); s.Status != domain.StepPending || s.PrivyActionID != "" {
		t.Fatalf("step = %+v, want pending and unsent", s)
	}
}

func TestBridge_aLegFromEVMNeedsNoSOL(t *testing.T) {
	t.Parallel()
	e := newBridgeEnv(t)
	e.leg.Spec.From, e.leg.Spec.To = chain.NetworkBase, chain.NetworkSolana
	e.leg.Ref.Step = domain.StepBridgeBack
	e.privy.script(multichain.ActionSucceeded)

	if _, err := e.run(t.Context()); err != nil || len(e.topUps.list()) != 0 {
		t.Fatalf("Run err = %v after %d top-ups, want none for an EVM source", err, len(e.topUps.list()))
	}
}
