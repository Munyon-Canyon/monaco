package treasury_test

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	memberAddress   = chain.SolanaAddress("9xQeWvG816bUx9EPjHmaT23yvVMvM9fQj4a8PHF4H6P")
	treasuryAddress = chain.SolanaAddress("5kwEmpcR8Txq1b4bDazRm9j4cx8Qo2aiE53rYA1dCDDP")
)

type fundStubs struct {
	member      bool
	status      cabalport.Status
	paused      bool
	onChain     money.Micros
	totalShares money.SharesUnits
	pot         money.Micros
	potErr      error
	buildErr    error
	broadcast   error
	reads       adapters.FundOutflows
	ownedAtSend []bool
	sent        int
	owner       *adapters.Queries
	failOn      string
	onBuild     func()
	signature   chain.Signature
	mu          sync.Mutex
}

func (s *fundStubs) err(method string) error {
	if s.failOn == method {
		return errs.New(errs.CodeInternal, "stub."+method)
	}
	return nil
}

func (s *fundStubs) IsMember(context.Context, ids.CabalID, ids.UserID) (bool, error) {
	return s.member, s.err("IsMember")
}

func (s *fundStubs) Status(context.Context, ids.CabalID) (cabalport.Status, error) {
	return s.status, s.err("Status")
}

func (s *fundStubs) TreasuryWallet(_ context.Context, id ids.CabalID) (cabalport.TreasuryWallet, error) {
	w := cabalport.TreasuryWallet{CabalID: id, PrivyWalletID: "treasury-wallet", Address: treasuryAddress}
	return w, s.err("TreasuryWallet")
}

func (s *fundStubs) MemberWallet(_ context.Context, user ids.UserID) (identityport.MemberWallet, error) {
	w := identityport.MemberWallet{UserID: user, PrivyWalletID: "member-wallet", Address: memberAddress}
	return w, s.err("MemberWallet")
}

func (s *fundStubs) Available(ctx context.Context, user ids.UserID) (fundingport.Balance, error) {
	if err := s.err("Available"); err != nil {
		return fundingport.Balance{}, err
	}
	inFlight, err := s.reads.InFlightMicros(ctx, user)
	if err != nil {
		return fundingport.Balance{}, err
	}
	available, err := s.onChain.Sub(inFlight)
	if err != nil {
		available = money.Micros{}
	}
	return fundingport.Balance{OnChainMicros: s.onChain, InFlightFundMicros: inFlight, AvailableMicros: available}, nil
}

func (s *fundStubs) IsPaused(context.Context, ids.CabalID) (fundingport.Pause, error) {
	return fundingport.Pause{Paused: s.paused}, s.err("IsPaused")
}

func (*fundStubs) PausedCabals(context.Context) (fundingport.PausedSet, error) {
	return fundingport.PausedSet{}, nil
}

func (s *fundStubs) TotalShares(context.Context, ids.CabalID) (money.SharesUnits, error) {
	return s.totalShares, s.err("TotalShares")
}

func (s *fundStubs) PotValue(context.Context, ids.CabalID) (money.Micros, error) {
	return s.pot, s.potErr
}

func (s *fundStubs) Build(_ context.Context, spec relayer.TransferSpec) (relayer.SignedTx, error) {
	if s.onBuild != nil {
		s.onBuild()
	}
	if s.buildErr != nil {
		return relayer.SignedTx{}, s.buildErr
	}
	if spec.FromWallet.Address != memberAddress || spec.To != treasuryAddress || spec.Amount.Decimals() != 6 {
		return relayer.SignedTx{}, errs.New(errs.CodeInvalidInput, "stub.Build")
	}
	sig := s.signature
	if sig == "" {
		sig = chain.Signature("sig-" + uuid.NewString())
	}
	return relayer.SignedTx{Bytes: []byte(sig), Signature: sig, LastValidBlockHeight: 900}, nil
}

func (s *fundStubs) Broadcast(ctx context.Context, tx relayer.SignedTx) error {
	owned, err := s.owner.OwnsSignature(ctx, tx.Signature)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ownedAtSend = append(s.ownedAtSend, owned && err == nil)
	s.sent++
	return s.broadcast
}

type fundHarness struct {
	fixture
	stubs   *fundStubs
	handler *app.FundCabalHandler
	user    ids.UserID
	cabal   ids.CabalID
}

func newFundHarness(t *testing.T) *fundHarness {
	t.Helper()
	return newFundHarnessHinting(t, &hints{})
}

func newFundHarnessHinting(t *testing.T, sent *hints) *fundHarness {
	t.Helper()
	f := newFixture(t)
	usdcAddress := chain.SolanaAddress(f.cfg.Solana.USDCMint)
	s := &fundStubs{
		member: true, status: cabalport.StatusActive, onChain: money.MicrosFromUint64(100_000_000),
		reads: adapters.NewFundOutflows(f.pool), owner: adapters.NewQueries(f.pool, nil, nil, f.clock, usdcAddress),
	}
	h := app.NewFundCabalHandler(app.FundCabalDeps{
		UoW: f.uow, IDs: f.ids, Clock: f.clock, Cabals: s, Wallets: s, Balances: s,
		Pauses: func(db.Tx) fundingport.Pauses { return s }, Pot: s, Transfers: s,
		Hints: sent, USDC: chain.Mint{Address: usdcAddress, Decimals: 6},
	})
	return &fundHarness{fixture: f, stubs: s, handler: h, user: f.user(t), cabal: f.cabal(t)}
}

func (h *fundHarness) actorCtx() context.Context {
	return auth.WithActor(h.ctx(), auth.Actor{Kind: auth.ActorUser, ID: h.user.String()})
}

func (h *fundHarness) fund(micros uint64) (uuid.UUID, error) {
	return h.handler.Handle(h.actorCtx(), app.FundCabal{
		CabalID: h.cabal, UserID: h.user, Amount: money.MicrosFromUint64(micros),
	})
}

func (h *fundHarness) transfer(t *testing.T, id uuid.UUID) sqlc.GetFundTransferRow {
	t.Helper()
	row, err := sqlc.New(h.pool).GetFundTransfer(h.ctx(), id)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func (h *fundHarness) events(t *testing.T, typ string) int {
	t.Helper()
	var n int
	if err := h.pool.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE type = $1`, typ).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestFundCabal_submitsAndStoresTheSignatureBeforeBroadcast(t *testing.T) {
	t.Parallel()
	h := newFundHarness(t)
	id, err := h.fund(5_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if got := h.transfer(t, id); got.Status != string(domain.FundSubmitted) || got.AmountMicros != "5000000" {
		t.Fatalf("transfer = %+v, want submitted 5000000", got)
	}
	if len(h.stubs.ownedAtSend) != 1 || !h.stubs.ownedAtSend[0] {
		t.Fatalf("owned at broadcast = %v, want [true]", h.stubs.ownedAtSend)
	}
	if n := h.events(t, "cabal.fund_submitted"); n != 1 {
		t.Fatalf("cabal.fund_submitted events = %d, want 1", n)
	}
	if n := h.count(t, "user_txns"); n != 0 {
		t.Fatalf("user_txns = %d, want 0 before the mint", n)
	}
}

func TestFundCabal_refusals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		amount uint64
		setup  func(*fundStubs)
		want   errs.Code
	}{
		{"not a member", 5_000_000, func(s *fundStubs) { s.member = false }, errs.CodeNotCabalMember},
		{"banned", 5_000_000, func(s *fundStubs) { s.status = cabalport.StatusBanned }, errs.CodeCabalPaused},
		{"paused", 5_000_000, func(s *fundStubs) { s.paused = true }, errs.CodeCabalPaused},
		{"over balance", 100_000_001, func(*fundStubs) {}, errs.CodeInsufficientFunds},
		{"pot has shares but no value", 5_000_000, func(s *fundStubs) {
			s.totalShares = money.SharesUnitsFromUint64(10)
		}, errs.CodePotValueZero},
		{"membership read fails", 5_000_000, func(s *fundStubs) { s.failOn = "IsMember" }, errs.CodeInternal},
		{"status read fails", 5_000_000, func(s *fundStubs) { s.failOn = "Status" }, errs.CodeInternal},
		{"member wallet read fails", 5_000_000, func(s *fundStubs) { s.failOn = "MemberWallet" }, errs.CodeInternal},
		{
			"treasury wallet read fails",
			5_000_000,
			func(s *fundStubs) { s.failOn = "TreasuryWallet" },
			errs.CodeInternal,
		},
		{"pause read fails", 5_000_000, func(s *fundStubs) { s.failOn = "IsPaused" }, errs.CodeInternal},
		{"balance read fails", 5_000_000, func(s *fundStubs) { s.failOn = "Available" }, errs.CodeInternal},
		{"share read fails", 5_000_000, func(s *fundStubs) { s.failOn = "TotalShares" }, errs.CodeInternal},
		{"pot read fails", 5_000_000, func(s *fundStubs) {
			s.totalShares, s.potErr = money.SharesUnitsFromUint64(10), errs.New(errs.CodeInternal, "stub")
		}, errs.CodeInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newFundHarness(t)
			tt.setup(h.stubs)
			_, err := h.fund(tt.amount)
			wantCode(t, err, tt.want)
			if n := h.count(t, "fund_transfers"); n != 0 || h.stubs.sent != 0 {
				t.Fatalf("fund_transfers = %d, broadcasts = %d, want none", n, h.stubs.sent)
			}
		})
	}
}

func TestFundCabal_priceUnavailableDoesNotRefuse(t *testing.T) {
	t.Parallel()
	h := newFundHarness(t)
	h.stubs.totalShares = money.SharesUnitsFromUint64(10)
	h.stubs.potErr = errs.New(errs.CodePriceUnavailable, "stub")
	if _, err := h.fund(5_000_000); err != nil {
		t.Fatalf("fund with no price = %v, want submitted", err)
	}
	h.stubs.potErr, h.stubs.pot = nil, money.MicrosFromUint64(1)
	if _, err := h.fund(5_000_000); err != nil {
		t.Fatalf("fund with a valued pot = %v, want submitted", err)
	}
}

func TestFundCabal_privyFailureFailsTheRowAndFreesTheBalance(t *testing.T) {
	t.Parallel()
	h := newFundHarness(t)
	h.stubs.buildErr = errs.New(errs.CodePrivyUnavailable, "stub")
	_, err := h.fund(60_000_000)
	wantCode(t, err, errs.CodePrivyUnavailable)
	var status, code string
	if err := h.pool.QueryRow(t.Context(), `SELECT status, fail_code FROM fund_transfers`).
		Scan(&status, &code); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || code != "privy_unavailable" || h.stubs.sent != 0 {
		t.Fatalf("row = %s/%s, broadcasts = %d, want failed/privy_unavailable and none", status, code, h.stubs.sent)
	}
	h.stubs.buildErr = nil
	if _, err := h.fund(60_000_000); err != nil {
		t.Fatalf("second fund = %v, want the failed row released", err)
	}
}

func TestFundCabal_broadcastErrorStillAnswersSubmitted(t *testing.T) {
	t.Parallel()
	h := newFundHarness(t)
	h.stubs.broadcast = errs.New(errs.CodeRPCUnavailable, "stub")
	id, err := h.fund(5_000_000)
	if err != nil || h.transfer(t, id).Status != string(domain.FundSubmitted) {
		t.Fatalf("fund = %v, want submitted for the poller to rebroadcast", err)
	}
}

func TestFundCabal_concurrentFundsNeverOverspend(t *testing.T) {
	t.Parallel()
	h := newFundHarness(t)
	var wg sync.WaitGroup
	results := make([]error, 2)
	for i := range results {
		wg.Go(func() { _, results[i] = h.fund(60_000_000) })
	}
	wg.Wait()
	ok, refused := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			ok++
		case errs.CodeOf(err) == errs.CodeInsufficientFunds:
			refused++
		default:
			t.Fatalf("unexpected error %v", err)
		}
	}
	if ok != 1 || refused != 1 {
		t.Fatalf("ok = %d, insufficient = %d, want one each", ok, refused)
	}
}

func TestFundCabal_aRowFailedWhileSigningIsNeverSent(t *testing.T) {
	t.Parallel()
	h := newFundHarness(t)
	h.stubs.onBuild = func() {
		if _, err := h.pool.Exec(context.Background(),
			`UPDATE fund_transfers SET status = 'failed', fail_code = 'fund_not_sent' WHERE status = 'created'`,
		); err != nil {
			t.Error(err)
		}
	}
	_, err := h.fund(5_000_000)
	wantCode(t, err, errs.CodeFundNotSent)
	if h.stubs.sent != 0 || h.events(t, "cabal.fund_submitted") != 0 {
		t.Fatalf("broadcasts = %d, want none for a failed row", h.stubs.sent)
	}
}

func TestFundCabal_aStoreFailureAfterSigningNeverBroadcasts(t *testing.T) {
	t.Parallel()
	h := newFundHarness(t)
	h.stubs.signature = "reused-signature"
	if _, err := h.fund(5_000_000); err != nil {
		t.Fatal(err)
	}
	if _, err := h.fund(5_000_000); err == nil {
		t.Fatal("second fund with a duplicate signature = nil error")
	}
	if h.stubs.sent != 1 {
		t.Fatalf("broadcasts = %d, want 1", h.stubs.sent)
	}
}

func TestFundCabal_aFailedFailWriteLeavesTheRowForThePoller(t *testing.T) {
	t.Parallel()
	h := newFundHarness(t)
	ctx, cancel := context.WithCancel(h.actorCtx())
	h.stubs.onBuild = cancel
	h.stubs.buildErr = errs.New(errs.CodePrivyUnavailable, "stub")
	_, err := h.handler.Handle(
		ctx,
		app.FundCabal{CabalID: h.cabal, UserID: h.user, Amount: money.MicrosFromUint64(5_000_000)},
	)
	wantCode(t, err, errs.CodePrivyUnavailable)
	var status string
	if err := h.pool.QueryRow(t.Context(), `SELECT status FROM fund_transfers`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "created" {
		t.Fatalf("status = %s, want created for the poller to fail", status)
	}
}

func TestFundCabal_hintsTheBalanceAfterTheSubmitCommits(t *testing.T) {
	t.Parallel()
	sent := &hints{}
	h := newFundHarnessHinting(t, sent)
	if _, err := h.fund(5_000_000); err != nil {
		t.Fatal(err)
	}
	if got, want := sent.sent(), []string{events.UserBalanceChangedHint(h.user)}; !slices.Equal(got, want) {
		t.Fatalf("hints = %q, want %q", got, want)
	}
}

func TestFundCabal_hintsNothingWhenTheSubmitRollsBack(t *testing.T) {
	t.Parallel()
	sent := &hints{}
	h := newFundHarnessHinting(t, sent)
	if _, err := h.pool.Exec(t.Context(), refuse("events", "NEW.type = 'cabal.fund_submitted'")); err != nil {
		t.Fatal(err)
	}
	if _, err := h.fund(5_000_000); err == nil {
		t.Fatal("fund error = nil, want the refused event to roll the submit back")
	}
	if got := sent.sent(); len(got) != 0 {
		t.Fatalf("hints = %q, want none after a rollback", got)
	}
}
