package funding_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	withdrawMicros = 2_000_000
	withdrawTo     = chain.SolanaAddress("9xQeWvG816bUx9EPjHmaT23yvVMvM9fQj4a8PHF4H6P")
	withdrawSig    = chain.Signature(
		"5VERv8NMvzbJMEkV8xnrLkEaWRtSz9CosKDYjCJjBRnbJLgp8uirBgmQpjKhoR4tjF3ZpRzrFmBV6UjKdiSZkQUW")
)

type signingWallet struct {
	wallet chain.Wallet
	err    error
}

func (w signingWallet) SigningWallet(context.Context, ids.UserID) (chain.Wallet, error) {
	return w.wallet, w.err
}

type chainBalance struct{ micros uint64 }

func (b chainBalance) TokenBalance(context.Context, chain.SolanaAddress, chain.Mint) (money.BaseUnits, error) {
	return money.NewBaseUnits(b.micros, 6), nil
}

type stubTransfers struct {
	mu         sync.Mutex
	builds     []relayer.TransferSpec
	buildErr   error
	beforeSign func()
	sendErr    error
	sent       []relayer.SignedTx
}

func (s *stubTransfers) Build(_ context.Context, spec relayer.TransferSpec) (relayer.SignedTx, error) {
	s.mu.Lock()
	s.builds = append(s.builds, spec)
	s.mu.Unlock()
	if s.beforeSign != nil {
		s.beforeSign()
	}
	if s.buildErr != nil {
		return relayer.SignedTx{}, s.buildErr
	}
	return relayer.SignedTx{Bytes: []byte{1, 2, 3}, Signature: withdrawSig, LastValidBlockHeight: 900}, nil
}

func (s *stubTransfers) Broadcast(_ context.Context, tx relayer.SignedTx) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, tx)
	return s.sendErr
}

type withdrawFixture struct {
	pool      *pgxpool.Pool
	user      testkit.SeededUser
	hints     *hints
	transfers *stubTransfers
	handler   *app.WithdrawHandler
	now       time.Time
	clock     *testkit.Clock
	uow       *db.UnitOfWork
}

func newWithdrawFixture(t *testing.T, onChain uint64, opts ...func(*app.WithdrawDeps)) *withdrawFixture {
	t.Helper()
	pool := testkit.DB(t)
	f := &withdrawFixture{
		pool: pool, user: testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true}),
		hints: &hints{}, transfers: &stubTransfers{}, now: clock.Real{}.Now().UTC().Truncate(time.Microsecond),
	}
	clk := testkit.NewClock(f.now)
	f.clock, f.uow = clk, db.New(pool, testkit.NewIDs(5), clk)
	usdc := chain.Mint{Address: testkit.USDCMint, Decimals: 6}
	deps := app.WithdrawDeps{
		UoW: f.uow,
		Balances: adapters.NewBalances(
			signingWalletAddress{f.user.Address},
			func() adapters.TokenBalances { return chainBalance{micros: onChain} },
			adapters.Outflows{
				Funds:       treasury.New(module.Deps{Pool: pool}).FundOutflows(),
				Withdrawals: app.WithdrawalOutflows{Reads: pool},
			},
			clk, usdc),
		Wallets:   signingWallet{wallet: chain.Wallet{ID: f.user.PrivyWalletID, Address: f.user.Address}},
		Transfers: func() (app.Transfers, error) { return f.transfers, nil },
		Hints:     f.hints, Clock: clk, USDC: usdc,
	}
	for _, opt := range opts {
		opt(&deps)
	}
	f.handler = app.NewWithdrawHandler(deps)
	return f
}

type signingWalletAddress struct{ address chain.SolanaAddress }

func (w signingWalletAddress) MemberWalletAddress(context.Context, ids.UserID) (chain.SolanaAddress, error) {
	return w.address, nil
}

func (f *withdrawFixture) withdraw(t *testing.T) (app.WithdrawResult, error) {
	t.Helper()
	return f.handle(f.ctx(t.Context()), withdrawTo)
}

func (f *withdrawFixture) handle(ctx context.Context, to chain.SolanaAddress) (app.WithdrawResult, error) {
	return f.handler.Handle(ctx, app.Withdraw{
		ID: ids.Real{}.NewV7(), UserID: f.user.ID,
		Request: domain.WithdrawalRequest{Amount: money.MicrosFromUint64(withdrawMicros), To: to},
	})
}

func (f *withdrawFixture) ctx(parent context.Context) context.Context {
	return observability.WithActor(parent, "user:"+f.user.ID.String())
}

type withdrawalRow struct {
	status, failCode, signature string
	signed                      []byte
	height                      *int64
}

func (f *withdrawFixture) rows(t *testing.T) []withdrawalRow {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT status, COALESCE(fail_code, ''), COALESCE(tx_signature, ''),
		signed_tx, last_valid_block_height FROM withdrawals ORDER BY created_at, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []withdrawalRow
	for rows.Next() {
		var r withdrawalRow
		if err := rows.Scan(&r.status, &r.failCode, &r.signature, &r.signed, &r.height); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func (f *withdrawFixture) submittedEvents(t *testing.T) []events.WithdrawalSubmitted {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT payload FROM events WHERE type = $1`, events.TypeWithdrawalSubmitted)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []events.WithdrawalSubmitted
	for rows.Next() {
		var raw []byte
		var e events.WithdrawalSubmitted
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &e); err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
	}
	return out
}

func TestWithdraw_signsStoresAndBroadcasts(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	got, err := f.withdraw(t)
	if err != nil || got.Status != domain.WithdrawalSubmitted || got.TxSignature != withdrawSig {
		t.Fatalf("Handle = %+v, %v", got, err)
	}
	assertSubmittedRow(t, f)
	assertSignedAndSent(t, f)
	want := events.WithdrawalSubmitted{
		V: 1, WithdrawalID: got.ID, UserID: f.user.ID.UUID(), AmountMicros: money.MicrosFromUint64(withdrawMicros),
		ToAddress: withdrawTo, TxSignature: withdrawSig,
	}
	if evs := f.submittedEvents(t); len(evs) != 1 || evs[0] != want {
		t.Fatalf("events = %+v, want %+v", evs, want)
	}
	if len(f.hints.keys) != 1 || f.hints.keys[0] != events.UserBalanceChangedHint(f.user.ID) {
		t.Fatalf("hints = %v", f.hints.keys)
	}
	inFlight, err := (app.WithdrawalOutflows{Reads: f.pool}).InFlightMicros(t.Context(), f.user.ID)
	if err != nil || inFlight.String() != "2000000" {
		t.Fatalf("in flight = %v, %v", inFlight, err)
	}
}

func assertSubmittedRow(t *testing.T, f *withdrawFixture) {
	t.Helper()
	rows := f.rows(t)
	if len(rows) != 1 || rows[0].status != "submitted" || rows[0].signature != string(withdrawSig) ||
		string(rows[0].signed) != "\x01\x02\x03" || rows[0].height == nil || *rows[0].height != 900 {
		t.Fatalf("rows = %+v", rows)
	}
}

func assertSignedAndSent(t *testing.T, f *withdrawFixture) {
	t.Helper()
	spec := f.transfers.builds[0]
	if spec.FromWallet.ID != f.user.PrivyWalletID || spec.To != withdrawTo ||
		spec.Amount != money.NewBaseUnits(withdrawMicros, 6) || spec.Mint.Address != testkit.USDCMint {
		t.Fatalf("spec = %+v", spec)
	}
	if len(f.transfers.sent) != 1 || f.transfers.sent[0].Signature != withdrawSig {
		t.Fatalf("sent = %+v", f.transfers.sent)
	}
}

func TestWithdraw_InsufficientFundsCountsInFlight(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 3_000_000)
	if _, err := f.withdraw(t); err != nil {
		t.Fatal(err)
	}
	_, err := f.withdraw(t)
	if errs.CodeOf(err) != errs.CodeInsufficientFunds {
		t.Fatalf("second Handle err = %v, want insufficient_funds", err)
	}
	if rows := f.rows(t); len(rows) != 1 || len(f.transfers.builds) != 1 {
		t.Fatalf("rows = %+v builds = %d", rows, len(f.transfers.builds))
	}
}

func TestWithdraw_RacingWithdrawalsSpendTheBalanceOnce(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 3_000_000)
	results := make([]error, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Go(func() { _, results[i] = f.withdraw(t) })
	}
	wg.Wait()
	failed := 0
	for _, err := range results {
		switch {
		case errs.CodeOf(err) == errs.CodeInsufficientFunds:
			failed++
		case err != nil:
			t.Fatalf("Handle err = %v", err)
		}
	}
	if failed != 1 || len(f.rows(t)) != 1 {
		t.Fatalf("errors = %v rows = %+v", results, f.rows(t))
	}
}

func TestWithdraw_ToOwnWallet(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	_, err := f.handle(f.ctx(t.Context()), f.user.Address)
	if errs.CodeOf(err) != errs.CodeWithdrawToOwnWallet || len(f.rows(t)) != 0 {
		t.Fatalf("Handle err = %v rows = %+v", err, f.rows(t))
	}
}

func TestWithdraw_SignFailureFailsTheRow(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	f.transfers.buildErr = errs.New(errs.CodePrivyUnavailable, "test")
	_, err := f.withdraw(t)
	if errs.CodeOf(err) != errs.CodePrivyUnavailable {
		t.Fatalf("Handle err = %v, want privy_unavailable", err)
	}
	if rows := f.rows(t); len(rows) != 1 || rows[0].status != "failed" || rows[0].failCode != "privy_unavailable" {
		t.Fatalf("rows = %+v", rows)
	}
	if len(f.hints.keys) != 2 || len(f.transfers.sent) != 0 || len(f.submittedEvents(t)) != 0 {
		t.Fatalf("hints = %v sent = %d", f.hints.keys, len(f.transfers.sent))
	}
	inFlight, err := (app.WithdrawalOutflows{Reads: f.pool}).InFlightMicros(t.Context(), f.user.ID)
	if err != nil || !inFlight.IsZero() {
		t.Fatalf("in flight = %v, %v", inFlight, err)
	}
}

func TestWithdraw_RowFailedWhileSigningIsNotSent(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	f.transfers.beforeSign = func() {
		if _, err := f.pool.Exec(context.Background(),
			`UPDATE withdrawals SET status = 'failed', fail_code = 'withdrawal_not_sent', completed_at = now()`,
		); err != nil {
			t.Error(err)
		}
	}
	_, err := f.withdraw(t)
	if errs.CodeOf(err) != errs.CodeInternal || len(f.transfers.sent) != 0 || len(f.submittedEvents(t)) != 0 {
		t.Fatalf("Handle err = %v sent = %d", err, len(f.transfers.sent))
	}
}

func TestWithdraw_BroadcastFailureStaysSubmitted(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	f.transfers.sendErr = errs.New(errs.CodeRPCUnavailable, "test")
	got, err := f.withdraw(t)
	if err != nil || got.Status != domain.WithdrawalSubmitted {
		t.Fatalf("Handle = %+v, %v", got, err)
	}
	if rows := f.rows(t); len(rows) != 1 || rows[0].status != "submitted" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestWithdraw_DependencyErrorsWriteNothing(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodePrivyUnavailable, "test")
	for name, opt := range map[string]func(*app.WithdrawDeps){
		"wallet":    func(d *app.WithdrawDeps) { d.Wallets = signingWallet{err: boom} },
		"transfers": func(d *app.WithdrawDeps) { d.Transfers = func() (app.Transfers, error) { return nil, boom } },
		"balance":   func(d *app.WithdrawDeps) { d.Balances = fixedBalancesErr{boom} },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newWithdrawFixture(t, 5_000_000, opt)
			if _, err := f.withdraw(t); !errors.Is(err, boom) || len(f.rows(t)) != 0 {
				t.Fatalf("Handle err = %v rows = %+v", err, f.rows(t))
			}
		})
	}
}

type fixedBalancesErr struct{ err error }

func (b fixedBalancesErr) Available(context.Context, ids.UserID) (funding.Balance, error) {
	return funding.Balance{}, b.err
}

func TestWithdraw_SignFailureAfterRowMovedKeepsTheCause(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	boom := errs.New(errs.CodePrivyUnavailable, "test")
	f.transfers.buildErr = boom
	f.transfers.beforeSign = func() {
		if _, err := f.pool.Exec(context.Background(),
			`UPDATE withdrawals SET status = 'failed', fail_code = 'withdrawal_not_sent', completed_at = now()`,
		); err != nil {
			t.Error(err)
		}
	}
	_, err := f.withdraw(t)
	if !errors.Is(err, boom) || len(f.hints.keys) != 1 {
		t.Fatalf("Handle err = %v hints = %v", err, f.hints.keys)
	}
	if rows := f.rows(t); rows[0].failCode != "withdrawal_not_sent" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestWithdraw_SignFailureWithDeadContextReportsBoth(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	ctx, cancel := context.WithCancel(t.Context())
	boom := errs.New(errs.CodePrivyUnavailable, "test")
	f.transfers.buildErr = boom
	f.transfers.beforeSign = cancel
	_, err := f.handle(f.ctx(ctx), withdrawTo)
	if !errors.Is(err, boom) || !errors.Is(err, context.Canceled) {
		t.Fatalf("Handle err = %v, want the sign error and the cancel", err)
	}
}
