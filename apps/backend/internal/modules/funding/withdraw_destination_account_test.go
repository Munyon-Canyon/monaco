package funding_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

type probingTransfers struct {
	*stubTransfers
	has   bool
	err   error
	asked []chain.SolanaAddress
}

func (p *probingTransfers) HasTokenAccount(_ context.Context, owner chain.SolanaAddress, _ chain.Mint) (bool, error) {
	p.asked = append(p.asked, owner)
	return p.has, p.err
}

func (f *withdrawFixture) confirmTo(t *testing.T, to chain.SolanaAddress) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(),
		`UPDATE withdrawals SET status = 'confirmed', completed_at = now() WHERE to_address = $1`, to); err != nil {
		t.Fatal(err)
	}
}

func newProbingFixture(t *testing.T, p *probingTransfers, opts ...func(*app.WithdrawDeps)) *withdrawFixture {
	t.Helper()
	probe := func(d *app.WithdrawDeps) { d.Transfers = func() (app.Transfers, error) { return p, nil } }
	f := newWithdrawFixture(t, 100_000_000, append([]func(*app.WithdrawDeps){probe}, opts...)...)
	p.stubTransfers = f.transfers
	return f
}

func TestWithdraw_ADestinationThatClosedItsAccountGetsItCreatedAgain(t *testing.T) {
	t.Parallel()
	p := &probingTransfers{has: true}
	f := newProbingFixture(t, p)
	to := freshDestination(0)
	if _, err := f.handle(f.ctx(t.Context()), to); err != nil {
		t.Fatal(err)
	}
	f.confirmTo(t, to)
	if _, err := f.handle(f.ctx(t.Context()), to); err != nil {
		t.Fatal(err)
	}
	f.wantSkipped(t, false, true)
	p.has = false
	if _, err := f.handle(f.ctx(t.Context()), to); err != nil {
		t.Fatal(err)
	}
	f.wantSkipped(t, false, true, false)
}

func TestWithdraw_AnUnreadableDestinationKeepsTheConfirmedShortcut(t *testing.T) {
	t.Parallel()
	p := &probingTransfers{err: errs.New(errs.CodeRPCUnavailable, "test")}
	f := newProbingFixture(t, p)
	to := freshDestination(0)
	if _, err := f.handle(f.ctx(t.Context()), to); err != nil {
		t.Fatal(err)
	}
	f.confirmTo(t, to)
	if _, err := f.handle(f.ctx(t.Context()), to); err != nil {
		t.Fatal(err)
	}
	f.wantSkipped(t, false, true)
}

func TestWithdraw_ADestinationWithoutAnAccountThePlanCannotCreateIsRefusedAndReleased(t *testing.T) {
	t.Parallel()
	p := &probingTransfers{has: false}
	f := newProbingFixture(t, p, userBudget(sponsorFee))
	f.transfers.buildErr = errs.New(errs.CodeWithdrawNoUSDCAccount, "test")
	_, err := f.handle(f.ctx(t.Context()), freshDestination(0))
	if errs.CodeOf(err) != errs.CodeWithdrawNoUSDCAccount {
		t.Fatalf("Handle err = %v, want %s", err, errs.CodeWithdrawNoUSDCAccount)
	}
	f.wantSkipped(t, true)
	rows := f.rows(t)
	if len(rows) != 1 || rows[0].status != "failed" || rows[0].failCode != string(errs.CodeWithdrawNoUSDCAccount) {
		t.Fatalf("rows = %+v, want one failed row with %s", rows, errs.CodeWithdrawNoUSDCAccount)
	}
	if live, released := f.reservations(t); live != 0 || released != 3 {
		t.Fatalf("reservations live %d released %d, want the fee reservation released", live, released)
	}
}
