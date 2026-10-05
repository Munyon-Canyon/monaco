package treasury_test

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	payoutTreasury = chain.SolanaAddress("9xQeWvG816bUx9EPjHmaT23yvVMvM9fQj4a8PHF4H6P")
	payoutMember   = chain.SolanaAddress("5kwEmpcR8Txq1b4bDazRm9j4cx8Qo2aiE53rYA1dCDDP")
)

func payoutSig(n int) chain.Signature { return chain.Signature(fmt.Sprintf("payout-signature-%d", n)) }

type payoutTransfers struct {
	mu       sync.Mutex
	built    []relayer.TransferSpec
	sent     []chain.Signature
	buildErr error
	sendErr  error
	onBuild  func()
}

func (s *payoutTransfers) Build(_ context.Context, spec relayer.TransferSpec) (relayer.SignedTx, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.buildErr != nil {
		return relayer.SignedTx{}, s.buildErr
	}
	if s.onBuild != nil {
		s.onBuild()
	}
	s.built = append(s.built, spec)
	return relayer.SignedTx{
		Bytes: []byte(payoutSig(len(s.built))), Signature: payoutSig(len(s.built)), LastValidBlockHeight: 100,
	}, nil
}

func (s *payoutTransfers) Broadcast(_ context.Context, tx relayer.SignedTx) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, tx.Signature)
	return s.sendErr
}

type payoutWallets struct{ err, memberErr error }

func (w payoutWallets) TreasuryWallet(context.Context, ids.CabalID) (chain.Wallet, error) {
	return chain.Wallet{ID: "treasury-wallet", Address: payoutTreasury}, w.err
}

func (w payoutWallets) MemberAddress(context.Context, ids.UserID) (chain.SolanaAddress, error) {
	return payoutMember, w.memberErr
}

func TestCashOutPayout_signsAndSendsOneTransferFromTheTreasuryToTheMember(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	r.chain.set(payoutSig(1), landed())
	r.mustAdvance(t)
	r.mustAdvance(t)
	r.wantJob(t, "completed", "")
	want := relayer.TransferSpec{
		FromWallet: chain.Wallet{ID: "treasury-wallet", Address: payoutTreasury}, To: payoutMember,
		Mint: chain.Mint{Address: usdcMint, Decimals: 6}, Amount: money.NewBaseUnits(50_000_000, 6),
	}
	if len(r.transfers.built) != 1 || r.transfers.built[0] != want ||
		!slices.Equal(r.transfers.sent, []chain.Signature{payoutSig(1)}) {
		t.Fatalf("built %+v, sent %v", r.transfers.built, r.transfers.sent)
	}
	r.noDrift(t)
}

func TestCashOutPayout_signsAgainOnlyAfterTheLastAttemptLapsed(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	r.chain.set(payoutSig(1), lapsed())
	r.chain.set(payoutSig(2), landed())
	r.mustAdvance(t)
	r.wantJob(t, "completed", "")
	if got := r.attempts(t); !slices.Equal(got, []string{"1:expired", "2:confirmed"}) || len(r.transfers.built) != 2 {
		t.Fatalf("attempts = %v, built %d", got, len(r.transfers.built))
	}
	r.noDrift(t)
}

func TestCashOutPayout_aMissingPayoutIsResentWithTheSameBytes(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	r.mustAdvance(t)
	r.chain.set(payoutSig(1), missing())
	r.mustAdvance(t)
	if len(r.transfers.built) != 1 || !slices.Equal(r.transfers.sent, []chain.Signature{payoutSig(1), payoutSig(1)}) {
		t.Fatalf("built %d, sent %v", len(r.transfers.built), r.transfers.sent)
	}
	r.wantJob(t, "paying", "")
}

func TestCashOutPayout_aBroadcastErrorLeavesTheChainToDecide(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	r.transfers.sendErr = errs.New(errs.CodeRPCUnavailable, "test.send")
	r.chain.set(payoutSig(1), landed())
	r.mustAdvance(t)
	r.wantJob(t, "completed", "")
}

func TestCashOutPayout_aTransferThatCannotBeBuiltFailsTheJob(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	r.transfers.buildErr = errs.New(errs.CodeInvalidAddress, "test.build")
	r.mustAdvance(t)
	r.wantJob(t, "failed", string(errs.CodePayoutFailed))
	if len(r.attempts(t)) != 0 || r.shares(t, r.alice) != 100 {
		t.Fatalf("attempts %v, alice shares %d", r.attempts(t), r.shares(t, r.alice))
	}
	r.noDrift(t)
}

func TestCashOutPayout_anOutageBeforeSigningIsRetried(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		rig  func(*payoutRig) *app.CashOutPayouts
		code errs.Code
	}{
		{"privy down", func(r *payoutRig) *app.CashOutPayouts {
			r.transfers.buildErr = errs.New(errs.CodePrivyUnavailable, "test.sign")
			return r.payouts()
		}, errs.CodePrivyUnavailable},
		{"treasury wallet down", func(r *payoutRig) *app.CashOutPayouts {
			r.wallets.err = errs.New(errs.CodeUpstreamUnavailable, "test.wallet")
			return r.payouts()
		}, errs.CodeUpstreamUnavailable},
		{"member without a wallet", func(r *payoutRig) *app.CashOutPayouts {
			r.wallets.memberErr = errs.New(errs.CodeNotFound, "test.member")
			return r.payouts()
		}, errs.CodeNotFound},
		{"transfers unbuilt", func(r *payoutRig) *app.CashOutPayouts {
			return app.NewCashOutPayouts(app.CashOutPayoutDeps{
				UoW: r.f.uow, Reads: r.f.pool, Ledger: r.f.ledger, IDs: r.f.ids, Clock: r.f.clock, Chain: r.chain,
				Transfers: func() (app.PayoutTransfers, error) {
					return nil, errs.New(errs.CodeInvalidInput, "test.config")
				},
			})
		}, errs.CodeInvalidInput},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := newPayoutRig(t)
			if err := c.rig(r).Advance(r.f.ctx(), r.job, 0, nil); errs.CodeOf(err) != c.code {
				t.Fatalf("Advance = %v, want %s", err, c.code)
			}
			r.wantJob(t, "started", "")
			if r.shares(t, r.alice) != 0 {
				t.Fatal("an outage returned the units")
			}
		})
	}
}

func TestCashOutPayout_aStoreFailureWhileSigningMovesNoMoney(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, setup, job string
		code             errs.Code
	}{
		{"attempt insert refused", refuse("cash_out_payouts", "NEW.status = 'signed'"), "started", errs.CodeInternal},
		{"attempt insert lost", skip("cash_out_payouts", "NEW.status = 'signed'"), "started", errs.CodeVersionConflict},
		{"start paying refused", refuse("cash_out_jobs", "NEW.status = 'paying'"), "started", errs.CodeInternal},
		{"start paying lost", skip("cash_out_jobs", "NEW.status = 'paying'"), "started", errs.CodeVersionConflict},
		{"broadcast mark refused", refuse("cash_out_payouts", "NEW.status = 'broadcast'"), "paying", errs.CodeInternal},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := newPayoutRig(t)
			if _, err := r.f.pool.Exec(t.Context(), c.setup); err != nil {
				t.Fatal(err)
			}
			err := r.advance(t, 0)
			if err == nil || (c.code != errs.CodeInternal && errs.CodeOf(err) != c.code) {
				t.Fatalf("Advance = %v, want %s", err, c.code)
			}
			r.wantJob(t, c.job, "")
		})
	}
}

func TestCashOutPayout_aJobMovedWhileSigningStoresNoAttempt(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	r.transfers.onBuild = func() {
		if _, err := r.f.pool.Exec(context.Background(), `UPDATE cash_out_jobs SET status = 'failed' WHERE id = $1`,
			r.job); err != nil {
			panic(err)
		}
	}
	r.mustAdvance(t)
	if len(r.attempts(t)) != 0 || len(r.transfers.sent) != 0 {
		t.Fatalf("attempts = %v, sent %v; nothing may be stored or sent", r.attempts(t), r.transfers.sent)
	}
}

func TestCashOutPayout_aJobThatMustSellFirstIsPaidOnlyOnceTheSaleSettles(t *testing.T) {
	t.Parallel()
	s := newSaleRig(t, 80_000_000)
	r := (&payoutRig{f: s.f, alice: s.alice, bob: s.bob, cabal: s.cabal, job: s.job}).stubs()
	r.mustAdvance(t)
	s.deliver(t, s.started)
	r.mustAdvance(t)
	if len(r.transfers.built) != 0 {
		t.Fatal("paid before the sale settled")
	}
	s.deliver(t, s.sold(s.f.ids.NewV7(), 600, 31_000_000, 1))
	r.chain.set(payoutSig(1), landed())
	r.mustAdvance(t)
	r.wantJob(t, "completed", "")
	if len(r.transfers.built) != 1 || r.transfers.built[0].Amount != money.NewBaseUnits(50_000_000, 6) {
		t.Fatalf("built %+v", r.transfers.built)
	}
	r.noDrift(t)
}
