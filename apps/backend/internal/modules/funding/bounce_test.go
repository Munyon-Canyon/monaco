package funding_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type bounceChain struct {
	mu          sync.Mutex
	before      func()
	onStatus    func()
	mintErr     error
	accountsErr error
	expired     bool
	hashErr     error
	closed      bool
	state       solana.State
	failed      bool
	err         error
	queries     int
}

func (c *bounceChain) MintConfig(_ context.Context, mint chain.SolanaAddress) (solana.MintConfig, error) {
	return solana.MintConfig{Mint: chain.Mint{Address: mint, Decimals: 6}, TokenProgram: chain.SPLProgram}, c.mintErr
}

func (c *bounceChain) BlockhashValid(context.Context, string) (bool, error) {
	return !c.expired, c.hashErr
}

func (c *bounceChain) Accounts(
	_ context.Context, addrs []chain.SolanaAddress, _ uint64,
) (uint64, []solana.TokenAccountState, error) {
	if c.before != nil {
		c.before()
	}
	return 1, []solana.TokenAccountState{{Address: addrs[0], Exists: !c.closed}}, c.accountsErr
}

func (c *bounceChain) SignatureStatuses(_ context.Context, sigs []chain.Signature) ([]solana.Status, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.queries++
	if c.onStatus != nil {
		c.onStatus()
	}
	if c.err != nil {
		return nil, c.err
	}
	return []solana.Status{{Signature: sigs[0], State: c.state, Failed: c.failed}}, nil
}

func bounceCtx(t *testing.T) context.Context {
	t.Helper()
	return auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorSystem, ID: "funding.bounce"})
}

type bounceTransfers struct {
	mu       sync.Mutex
	builds   []relayer.TransferSpec
	sent     []relayer.SignedTx
	onBuild  func()
	buildErr error
	sendErr  error
}

func (b *bounceTransfers) Build(_ context.Context, spec relayer.TransferSpec) (relayer.SignedTx, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.builds = append(b.builds, spec)
	if b.onBuild != nil {
		b.onBuild()
	}
	sig := chain.Signature("bounce-" + strconv.Itoa(len(b.builds)))
	msg := relayer.TransferMessage(withdrawTo, spec, chain.SPLProgram, [32]byte{1})
	raw := chain.Transaction{Signatures: [][]byte{make([]byte, 64), make([]byte, 64)}, Message: msg}.Encode()
	return relayer.SignedTx{Bytes: raw, Signature: sig, LastValidBlockHeight: 900}, b.buildErr
}

func (b *bounceTransfers) Broadcast(_ context.Context, tx relayer.SignedTx) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sent = append(b.sent, tx)
	return b.sendErr
}

type steppingClock struct{ *testkit.Clock }

func (c steppingClock) After(d time.Duration) <-chan time.Time {
	c.Advance(d)
	ch := make(chan time.Time, 1)
	ch <- c.Now()
	return ch
}

type bounceTreasury struct {
	address chain.SolanaAddress
	err     error
}

func (b bounceTreasury) TreasuryWallet(context.Context, ids.CabalID) (cabalport.TreasuryWallet, error) {
	return cabalport.TreasuryWallet{PrivyWalletID: "treasury-wallet", Address: b.address}, b.err
}

type bounceFixture struct {
	*flow08
	chain     *bounceChain
	transfers *bounceTransfers
	deps      app.BounceDeps
	bouncer   *app.Bouncer
}

func newBounceFixture(t *testing.T) *bounceFixture {
	t.Helper()
	f := &bounceFixture{flow08: newFlow08(t), transfers: &bounceTransfers{}}
	f.chain = &bounceChain{state: solana.StateFinalized}
	failed, err := noop.NewMeterProvider().Meter("test").Int64Counter("funding_bounce_failed_total")
	if err != nil {
		t.Fatal(err)
	}
	f.deps = app.BounceDeps{
		UoW: db.New(f.pool, testkit.NewIDs(81), f.clock), Reads: f.pool, Clock: steppingClock{f.clock}, Hints: &hints{},
		Chain: f.chain, Treasuries: bounceTreasury{address: flow08Treasury},
		Transfers: func() (app.Transfers, error) { return f.transfers, nil },
		Failed:    failed,
	}
	f.bouncer = app.NewBouncer(f.deps)
	return f
}

func (f *bounceFixture) detected(t *testing.T, sig chain.Signature) uuid.UUID {
	t.Helper()
	if err := f.deliver(t, sig); err != nil {
		t.Fatal(err)
	}
	var id uuid.UUID
	if err := f.pool.QueryRow(t.Context(), `SELECT id FROM external_deposits WHERE signature = $1`,
		string(sig)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *bounceFixture) status(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var status string
	if err := f.pool.QueryRow(t.Context(), `SELECT status FROM external_deposits WHERE id = $1`, id).
		Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func (f *bounceFixture) requireOneTransferBack(t *testing.T) {
	t.Helper()
	if len(f.transfers.builds) != 1 || len(f.transfers.sent) != 1 {
		t.Fatalf("builds = %d, sends = %d, want one each", len(f.transfers.builds), len(f.transfers.sent))
	}
	spec := f.transfers.builds[0]
	if spec.To != flow08Sender || spec.Amount.Uint64() != 25_000_000 || spec.FromWallet.Address != flow08Treasury ||
		spec.Mint.Address != testkit.USDCMint {
		t.Fatalf("spec = %+v, want 25 USDC from the treasury back to the sender", spec)
	}
}

func (f *bounceFixture) ledgerRows(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(t.Context(),
		`SELECT (SELECT count(*) FROM cabal_txns) + (SELECT count(*) FROM user_txns)`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestBounce_ReturnsTheDepositAndEndsThePause(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	id := f.detected(t, flow08USDCSig)

	first, again := f.bouncer.Start(bounceCtx(t), id), f.bouncer.Start(bounceCtx(t), id)

	if first != nil || again != nil {
		t.Fatalf("Start = %v then %v, want nil twice", first, again)
	}
	if got := f.status(t, id); got != "returned" {
		t.Fatalf("status = %s, want returned", got)
	}
	f.requireOneTransferBack(t)
	if externalPauses(t, f.pool) != 0 || countEvents(t, f.pool, events.TypeCabalResumed) != 1 ||
		countEvents(t, f.pool, events.TypeCabalExternalDepositBounced) != 1 {
		t.Fatal("want the pause resolved, one cabal.resumed and one cabal.external_deposit_bounced")
	}
	if f.ledgerRows(t) != 0 {
		t.Fatal("the bounce wrote ledger rows")
	}
}

func TestBounce_ClosedRecipientAccountFailsWithoutSigning(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	f.chain.closed = true
	id := f.detected(t, flow08USDCSig)

	if err := f.bouncer.Start(bounceCtx(t), id); err != nil {
		t.Fatal(err)
	}
	if got := f.status(t, id); got != "bounce_failed" || len(f.transfers.builds) != 0 {
		t.Fatalf("status = %s, builds = %d, want bounce_failed with no transfer", got, len(f.transfers.builds))
	}
	if externalPauses(t, f.pool) != 1 || countEvents(t, f.pool, events.TypeCabalResumed) != 0 {
		t.Fatal("a failed bounce ended the pause")
	}
}

func TestBounce_LeavesARowAnotherWriterMovedFirst(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	f.chain.closed = true
	id := f.detected(t, flow08USDCSig)
	f.chain.before = func() { exec(t, f.pool, `UPDATE external_deposits SET status = 'held'`) }

	if err := f.bouncer.Start(bounceCtx(t), id); err != nil {
		t.Fatal(err)
	}
	if got := f.status(t, id); got != "held" {
		t.Fatalf("status = %s, want the other writer's held", got)
	}
}

func TestBounce_UnreadableRowsAreErrors(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	id := f.detected(t, flow08USDCSig)
	exec(t, f.pool, `UPDATE external_deposits SET amount = 99999999999999999999`)

	decode, missing := f.bouncer.Start(bounceCtx(t), id), f.bouncer.Start(bounceCtx(t), ids.Real{}.NewV7())

	if errs.CodeOf(decode) != errs.CodeDecodeFailed || errs.CodeOf(missing) != errs.CodeInternal {
		t.Fatalf("Start = %v and %v, want decode_failed and internal", decode, missing)
	}
}

func TestBounce_ChainErrorFails(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	f.chain.failed = true
	id := f.detected(t, flow08USDCSig)

	if err := f.bouncer.Start(bounceCtx(t), id); err != nil {
		t.Fatal(err)
	}
	if got := f.status(t, id); got != "bounce_failed" || externalPauses(t, f.pool) != 1 {
		t.Fatalf("status = %s, want bounce_failed with the pause kept", got)
	}
}

func TestBounce_StillProcessingAfterTheWaitStaysBouncing(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	f.chain.state = solana.StateProcessing
	id := f.detected(t, flow08USDCSig)

	if err := f.bouncer.Start(bounceCtx(t), id); err != nil {
		t.Fatal(err)
	}
	if got := f.status(t, id); got != "bouncing" || f.chain.queries < 2 {
		t.Fatalf("status = %s after %d polls, want bouncing after polling", got, f.chain.queries)
	}
}

func TestBounce_RPCDownNaksAndARedeliveryLeavesItToTheSweeper(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	f.chain.err = errs.New(errs.CodeRPCUnavailable, "test")
	id := f.detected(t, flow08USDCSig)

	err := f.bouncer.Start(bounceCtx(t), id)

	if !errs.Retryable(errs.CodeOf(err)) {
		t.Fatalf("Start = %v, want a retryable error", err)
	}
	if err := f.bouncer.Start(bounceCtx(t), id); err != nil || len(f.transfers.builds) != 1 {
		t.Fatalf("redelivery = %v with %d builds, want an ack and no second transfer", err, len(f.transfers.builds))
	}
}

func TestBounce_PauseEndsOnlyAfterTheLastStrayTransferReturns(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	first, second := f.detected(t, flow08USDCSig), f.detected(t, flow08StockSigListed(t, f))

	if err := f.bouncer.Start(bounceCtx(t), first); err != nil {
		t.Fatal(err)
	}
	if externalPauses(t, f.pool) != 1 || countEvents(t, f.pool, events.TypeCabalResumed) != 0 {
		t.Fatal("the cabal resumed with a stray transfer still open")
	}
	if err := f.bouncer.Start(bounceCtx(t), second); err != nil {
		t.Fatal(err)
	}
	if externalPauses(t, f.pool) != 0 || countEvents(t, f.pool, events.TypeCabalResumed) != 1 {
		t.Fatal("want the cabal resumed once both bounces returned")
	}
}

func flow08StockSigListed(t *testing.T, f *bounceFixture) chain.Signature {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name,
		issuer_tradable, company_key, first_seen_at, updated_at)
		VALUES ($1, 'AAPLx', $2, 8, 'xstocks', 'equity', 'Apple', true, 'apple', now(), now())`,
		ids.Real{}.NewV7(), string(flow08XStock)); err != nil {
		t.Fatal(err)
	}
	return flow08StockSig
}

func TestNextBounceStatus(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		from domain.ExternalDepositStatus
		ev   domain.BounceEvent
		to   domain.ExternalDepositStatus
	}{
		{domain.ExternalDetected, domain.BounceSign, domain.ExternalBouncing},
		{domain.ExternalDetected, domain.BounceFail, domain.ExternalBounceFailed},
		{domain.ExternalDetected, domain.BounceHold, domain.ExternalHeld},
		{domain.ExternalBouncing, domain.BounceConfirm, domain.ExternalReturned},
		{domain.ExternalBouncing, domain.BounceFail, domain.ExternalBounceFailed},
		{domain.ExternalBounceFailed, domain.BounceRetry, domain.ExternalBouncing},
		{domain.ExternalBounceFailed, domain.BounceHold, domain.ExternalHeld},
	} {
		if got, err := domain.Next(tc.from, tc.ev); err != nil || got != tc.to {
			t.Errorf("Next(%s, %s) = %s, %v, want %s", tc.from, tc.ev, got, err, tc.to)
		}
	}
	for _, from := range []domain.ExternalDepositStatus{
		domain.ExternalReturned, domain.ExternalHeld, domain.ExternalIgnoredDust, domain.ExternalIgnoredUnknown,
	} {
		for _, ev := range []domain.BounceEvent{
			domain.BounceSign, domain.BounceConfirm, domain.BounceFail, domain.BounceRetry, domain.BounceHold,
		} {
			if _, err := domain.Next(from, ev); errs.CodeOf(err) != errs.CodeVersionConflict {
				t.Errorf("Next(%s, %s) = %v, want a version conflict", from, ev, err)
			}
		}
	}
	if _, err := domain.Next(domain.ExternalBouncing, domain.BounceHold); err == nil {
		t.Error("a bouncing deposit can be held")
	}
}
