package funding_test

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const depositSignature = "5VERv8NMvzbJMEkV8xnrLkEaWRtSz9CosKDYjCJjBRnbJLgp8uirBgmQpjKhoR4tjF3ZpRzrFmBV6UjKdiSZkQUW"

func depositBlockTime() time.Time { return time.Unix(1_790_000_000, 0).UTC() }

type depositRPC struct {
	signatures    []solana.SignatureInfo
	transfers     []solana.Transfer
	err           error
	signErr       error
	transferErr   error
	signCalls     int
	transferCalls int
}

func (r *depositRPC) SignaturesFor(
	context.Context,
	chain.SolanaAddress,
	chain.Signature,
	chain.Signature,
	int,
) ([]solana.SignatureInfo, error) {
	r.signCalls++
	if r.signErr != nil {
		return nil, r.signErr
	}
	return r.signatures, r.err
}

func (r *depositRPC) InboundTransfersForMint(
	context.Context,
	chain.Signature,
	chain.SolanaAddress,
	chain.SolanaAddress,
) ([]solana.Transfer, error) {
	r.transferCalls++
	if r.transferErr != nil {
		return nil, r.transferErr
	}
	return r.transfers, r.err
}

func TestDepositPollerReportsSignatureAndTransferFailures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		rpc  depositRPC
	}{
		{"signatures", depositRPC{signErr: errs.New(errs.CodeRPCUnavailable, "test.rpc")}},
		{"transfers", depositRPC{signatures: []solana.SignatureInfo{{Signature: depositSignature, Slot: 42, BlockTime: depositBlockTime()}}, transferErr: errs.New(errs.CodeRPCUnavailable, "test.rpc")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := testkit.DB(t)
			user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
			now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
			p := newDepositPoller(pool, user, now, &tc.rpc, 31, 32)
			if _, err := p.Tick(t.Context()); err == nil {
				t.Fatal("Tick error = nil")
			}
		})
	}
}

func unlimited() *rate.Limiter { return rate.NewLimiter(rate.Inf, 0) }

type rpcBudget struct {
	mu   sync.Mutex
	left int
}

func (b *rpcBudget) refill(n int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.left = n
}

func (b *rpcBudget) Wait(context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.left == 0 {
		return errs.New(errs.CodeInternal, "test.rpcBudget")
	}
	b.left--
	return nil
}

func newDepositPoller(
	pool *pgxpool.Pool,
	user testkit.SeededUser,
	now time.Time,
	rpc *depositRPC,
	uowID, pollerID uint64,
) *app.DepositPoller {
	return app.NewDepositPoller(
		pool, db.New(pool, testkit.NewIDs(uowID), testkit.NewClock(now)),
		testkit.NewIDs(pollerID), testkit.NewClock(now),
		fakes.NewIdentity(nil, []identity.MemberWallet{{UserID: user.ID, Address: user.Address}}),
		rpc, testkit.USDCMint, app.DepositPollInterval, unlimited(), &hints{},
	)
}

func TestDepositPollerAdvancesFailedSignatureAndRejectsOverflow(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	rpc := depositRPC{signatures: []solana.SignatureInfo{{
		Signature: depositSignature, Slot: 42, Failed: true, BlockTime: depositBlockTime(),
	}}}
	p := newDepositPoller(pool, user, now, &rpc, 33, 34)
	if report, err := p.Tick(t.Context()); err != nil || report.Changed != 0 {
		t.Fatalf("failed Tick = %+v, %v", report, err)
	}
	rpc.signatures = []solana.SignatureInfo{{Signature: "overflow", Slot: 43, BlockTime: depositBlockTime()}}
	rpc.transfers = []solana.Transfer{
		{Mint: chain.Mint{Address: testkit.USDCMint, Decimals: 6}, Net: money.NewBaseUnits(math.MaxUint64, 6)},
		{Mint: chain.Mint{Address: testkit.USDCMint, Decimals: 6}, Net: money.NewBaseUnits(1, 6)},
	}
	if _, err := p.Tick(t.Context()); err == nil {
		t.Fatal("overflow Tick error = nil")
	}
}

func TestDepositPollerReportsCreditFailureWithoutActor(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	rpc := depositRPC{
		signatures: []solana.SignatureInfo{{Signature: depositSignature, Slot: 42, BlockTime: depositBlockTime()}},
		transfers: []solana.Transfer{
			{Mint: chain.Mint{Address: testkit.USDCMint, Decimals: 6}, Net: money.NewBaseUnits(1, 6)},
		},
	}
	p := newDepositPoller(pool, user, now, &rpc, 35, 36)
	if _, err := p.Tick(t.Context()); err == nil {
		t.Fatal("Tick error = nil")
	}
}

func TestDepositPollerTouchesCursorAfterAnEmptyScan(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	rpc := depositRPC{}
	p := newDepositPoller(pool, user, now, &rpc, 37, 38)
	ctx := observability.WithActor(t.Context(), "system:poller.funding.deposits")
	if report, err := p.Tick(ctx); err != nil || report.Changed != 0 {
		t.Fatalf("Tick = %+v, %v", report, err)
	}
	var scannedAt time.Time
	const cursorScan = `SELECT scanned_at FROM deposit_cursors WHERE wallet_address = $1`
	err := pool.QueryRow(ctx, cursorScan, user.Address).Scan(&scannedAt)
	if err != nil || !scannedAt.Equal(now) {
		t.Fatalf("scanned_at = %s, %v; want %s", scannedAt, err, now)
	}
}

func TestDepositPoller_creditsUSDCAndAdvancesPastOtherTokens(t *testing.T) {
	t.Parallel()
	testDepositPollerAdvancesPastOtherTokens(t)
}

func testDepositPollerAdvancesPastOtherTokens(t *testing.T) {
	t.Helper()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC()
	rpc := depositRPC{
		signatures: []solana.SignatureInfo{{Signature: depositSignature, Slot: 42, BlockTime: depositBlockTime()}},
		transfers: []solana.Transfer{{
			Mint: chain.Mint{Address: "not-usdc", Decimals: 6}, Net: money.NewBaseUnits(4, 6),
		}},
	}
	p := app.NewDepositPoller(
		pool,
		db.New(pool, testkit.NewIDs(11), testkit.NewClock(now)),
		testkit.NewIDs(12),
		testkit.NewClock(now),
		fakes.NewIdentity(nil, []identity.MemberWallet{{UserID: user.ID, Address: user.Address}}),
		&rpc,
		testkit.USDCMint,
		app.DepositPollInterval,
		unlimited(),
		&hints{},
	)
	report, err := p.Tick(t.Context())
	if err != nil || report.Scanned != 1 || report.Changed != 0 {
		t.Fatalf("Tick = %+v, %v calls=%d/%d", report, err, rpc.signCalls, rpc.transferCalls)
	}
	var cursor string
	const readCursor = `SELECT last_signature FROM deposit_cursors WHERE wallet_address = $1`
	err = pool.QueryRow(t.Context(), readCursor, user.Address).Scan(&cursor)
	if err != nil || cursor != depositSignature {
		t.Fatalf("cursor = %q, %v", cursor, err)
	}
}

func TestDepositPoller_creditsOneDepositOnlyOnce(t *testing.T) {
	t.Parallel()
	testDepositPollerCreditsOneDepositOnlyOnce(t)
}

func testDepositPollerCreditsOneDepositOnlyOnce(t *testing.T) {
	t.Helper()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC()
	rpc := depositRPC{
		signatures: []solana.SignatureInfo{{Signature: depositSignature, Slot: 42, BlockTime: depositBlockTime()}},
		transfers: []solana.Transfer{{
			Mint: chain.Mint{Address: testkit.USDCMint, Decimals: 6}, Net: money.NewBaseUnits(25_000_000, 6),
		}},
	}
	p := app.NewDepositPoller(
		pool,
		db.New(pool, testkit.NewIDs(13), testkit.NewClock(now)),
		testkit.NewIDs(14),
		testkit.NewClock(now),
		fakes.NewIdentity(nil, []identity.MemberWallet{{UserID: user.ID, Address: user.Address}}),
		&rpc,
		testkit.USDCMint,
		app.DepositPollInterval,
		unlimited(),
		&hints{},
	)
	ctx := observability.WithActor(t.Context(), "system:poller.funding.deposits")
	if report, err := p.Tick(ctx); err != nil || report.Changed != 1 {
		t.Fatalf("first Tick = %+v, %v calls=%d/%d", report, err, rpc.signCalls, rpc.transferCalls)
	}
	if report, err := p.Tick(ctx); err != nil || report.Changed != 0 {
		t.Fatalf("second Tick = %+v, %v", report, err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM deposits`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("deposits = %d, %v", count, err)
	}
}

func TestDepositPoller_creditsWhenRPCOmitsBlockTime(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	rpc := depositRPC{
		signatures: []solana.SignatureInfo{{Signature: depositSignature, Slot: 42}},
		transfers: []solana.Transfer{{
			Mint: chain.Mint{Address: testkit.USDCMint, Decimals: 6}, Net: money.NewBaseUnits(1, 6),
		}},
	}
	p := newDepositPoller(pool, user, now, &rpc, 39, 40)
	ctx := observability.WithActor(t.Context(), "system:poller.funding.deposits")
	if report, err := p.Tick(ctx); err != nil || report.Changed != 1 {
		t.Fatalf("Tick = %+v, %v", report, err)
	}
	var blockTime *time.Time
	err := pool.QueryRow(ctx, `SELECT block_time FROM deposits WHERE tx_signature = $1`, depositSignature).
		Scan(&blockTime)
	if err != nil {
		t.Fatal(err)
	}
	if blockTime != nil {
		t.Fatalf("block_time = %s, want NULL", blockTime)
	}
}

type emptyRPC struct{}

func (emptyRPC) SignaturesFor(
	context.Context, chain.SolanaAddress, chain.Signature, chain.Signature, int,
) ([]solana.SignatureInfo, error) {
	return nil, nil
}

func (emptyRPC) InboundTransfersForMint(
	context.Context, chain.Signature, chain.SolanaAddress, chain.SolanaAddress,
) ([]solana.Transfer, error) {
	return nil, nil
}

func TestDepositPollerDefersWalletsPastTheRateBudgetWithoutFailingTheTick(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	wallets := make([]identity.MemberWallet, 0, 4)
	addrs := make([]string, 0, 4)
	for range 4 {
		user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
		wallets = append(wallets, identity.MemberWallet{UserID: user.ID, Address: user.Address})
		addrs = append(addrs, string(user.Address))
	}
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	budget := &rpcBudget{}
	p := app.NewDepositPoller(
		pool, db.New(pool, testkit.NewIDs(51), testkit.NewClock(now)), testkit.NewIDs(52), testkit.NewClock(now),
		fakes.NewIdentity(nil, wallets), emptyRPC{}, testkit.USDCMint, app.DepositPollInterval, budget, &hints{},
	)
	tick := func() {
		t.Helper()
		budget.refill(1)
		ctx := observability.WithActor(t.Context(), "system:poller.funding.deposits")
		if report, err := p.Tick(ctx); err != nil || report.Scanned != 1 {
			t.Fatalf("Tick = %+v, %v; want one wallet scanned and no error", report, err)
		}
	}
	scanned := func() int {
		t.Helper()
		var n int
		const q = `SELECT count(*) FROM deposit_cursors WHERE wallet_address = ANY($1)`
		if err := pool.QueryRow(t.Context(), q, addrs).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	tick()
	if got := scanned(); got != 1 {
		t.Fatalf("cursors after first tick = %d, want 1: deferred wallets keep no cursor so they sort first", got)
	}
	tick()
}
