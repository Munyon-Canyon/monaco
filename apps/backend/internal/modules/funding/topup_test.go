package funding_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

type topUpFake struct {
	mu      sync.Mutex
	err     error
	before  func() int
	wallets []chain.SolanaAddress
	at      []int
}

func (f *topUpFake) TopUpSOL(_ context.Context, wallet chain.SolanaAddress) error {
	at := -1
	if f.before != nil {
		at = f.before()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.wallets, f.at = append(f.wallets, wallet), append(f.at, at)
	return f.err
}

func (p *privyActions) transfers() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

func TestWithdrawEVM_topsUpTheMembersSolanaWalletBeforeTheTransfer(t *testing.T) {
	t.Parallel()
	actions := &privyActions{}
	f := newEVMFixture(t, actions)
	f.topUps.before = actions.transfers

	if _, err := f.withdrawEVM(f.ctx(t.Context())); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(f.topUps.wallets, []chain.SolanaAddress{f.user.Address}) || !slices.Equal(f.topUps.at, []int{0}) {
		t.Fatalf("top-ups %v at transfer counts %v, want the member wallet once before the transfer",
			f.topUps.wallets, f.topUps.at)
	}
}

func TestWithdrawEVM_aTopUpFailureLeavesTheWithdrawalUnsentForThePoller(t *testing.T) {
	t.Parallel()
	actions := &privyActions{}
	f := newEVMFixture(t, actions)
	f.topUps.err = errs.New(errs.CodeUpstreamUnavailable, "relayer.TopUps.TopUpSOL")

	if _, err := f.withdrawEVM(f.ctx(t.Context())); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("Handle err = %v, want the top-up error", err)
	}
	if row := f.evmRow(
		t,
	); row.status != string(domain.WithdrawalCreated) || row.action != "" ||
		actions.transfers() != 0 {
		t.Fatalf("row %+v after %d transfers, want created and unsent", row, actions.transfers())
	}

	f.clock.Advance(withdrawalUnsentAge + time.Second)
	if _, err := f.tick(
		t,
		f.evmPoller(actions),
	); errs.CodeOf(err) != errs.CodeUpstreamUnavailable ||
		actions.transfers() != 0 {
		t.Fatalf("Tick err = %v after %d transfers, want the poller held back by the same top-up", err,
			actions.transfers())
	}
	f.topUps.err, f.topUps.before = nil, actions.transfers
	if changed, err := f.tick(t, f.evmPoller(actions)); err != nil || changed != 1 {
		t.Fatalf("Tick = %d, %v, want the withdrawal sent once SOL is there", changed, err)
	}
	if at := f.topUps.at[len(f.topUps.at)-1]; at != 0 || actions.transfers() != 1 {
		t.Fatalf("last top-up at transfer count %d, %d transfers, want the top-up before the one send", at,
			actions.transfers())
	}
}
