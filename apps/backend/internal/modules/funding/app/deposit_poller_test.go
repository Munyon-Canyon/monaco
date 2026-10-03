package app

import (
	"context"
	"math"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type testWallets struct {
	wallets []port.MemberWallet
	pages   [][]port.MemberWallet
	called  int
	err     error
}

func (*testWallets) MemberWallet(context.Context, ids.UserID) (port.MemberWallet, error) {
	return port.MemberWallet{}, nil
}

func (w *testWallets) MemberWallets(context.Context, ids.UserID, int) ([]port.MemberWallet, error) {
	if w.err != nil {
		return nil, w.err
	}
	if len(w.pages) > 0 {
		page := w.pages[w.called]
		w.called++
		return page, nil
	}
	return w.wallets, nil
}

type pollerRPC struct{ pages [][]solana.SignatureInfo }

func (r *pollerRPC) SignaturesFor(
	_ context.Context,
	_ chain.SolanaAddress,
	_ chain.Signature,
	_ chain.Signature,
	_ int,
) ([]solana.SignatureInfo, error) {
	page := r.pages[0]
	r.pages = r.pages[1:]
	return page, nil
}

func (*pollerRPC) InboundTransfersForMint(
	context.Context, chain.Signature, chain.SolanaAddress, chain.SolanaAddress,
) ([]solana.Transfer, error) {
	return nil, nil
}

func TestDepositPollerPaginatesAndHonorsCancellation(t *testing.T) {
	t.Parallel()
	first := make([]solana.SignatureInfo, 1000)
	first[999].Signature = "before"
	rpc := &pollerRPC{pages: [][]solana.SignatureInfo{first, {}}}
	p := &DepositPoller{rpc: rpc, limit: rate.NewLimiter(rate.Inf, 1)}
	if got, err := p.signaturesSince(t.Context(), "wallet", "until"); err != nil || len(got) != 1000 {
		t.Fatalf("signaturesSince = %d, %v", len(got), err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	p.limit = rate.NewLimiter(rate.Limit(1), 0)
	if _, err := p.signaturesSince(ctx, "wallet", "until"); err == nil {
		t.Fatal("cancelled signaturesSince error = nil")
	}
	if _, err := p.scanSignature(ctx, port.MemberWallet{}, solana.SignatureInfo{}); err == nil {
		t.Fatal("cancelled scanSignature error = nil")
	}
}

func TestNewRPCLimiterDefaultsToTwentyPerSecond(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in    int32
		limit rate.Limit
		burst int
	}{{0, 20, 20}, {-1, 20, 20}, {7, 7, 7}} {
		if l := NewRPCLimiter(tc.in); l.Limit() != tc.limit || l.Burst() != tc.burst {
			t.Fatalf("NewRPCLimiter(%d) = %v/%d, want %v/%d", tc.in, l.Limit(), l.Burst(), tc.limit, tc.burst)
		}
	}
	if p := NewDepositPoller(nil, nil, nil, nil, nil, nil, "usdc", time.Second, nil, nil); p.Interval() != time.Second {
		t.Fatalf("interval = %s", p.Interval())
	}
}

func TestDepositPollerPagesWalletsAndReportsCursorAndLimiterFailures(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	p := &DepositPoller{
		reads: pool,
		wallets: &testWallets{wallets: []port.MemberWallet{
			{UserID: user.ID, Address: user.Address},
		}},
	}
	other := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	p.wallets = &testWallets{wallets: []port.MemberWallet{
		{UserID: user.ID, Address: user.Address},
		{UserID: other.ID, Address: other.Address},
	}}
	const insertCursor = `INSERT INTO deposit_cursors (wallet_address, last_signature, cursor_slot, scanned_at) VALUES ($1, '', 0, now())`
	if _, err := pool.Exec(t.Context(), insertCursor, user.Address); err != nil {
		t.Fatal(err)
	}
	if got, err := p.memberWallets(t.Context()); err != nil || got[0].Address != other.Address {
		t.Fatalf("ordered wallets = %+v, %v", got, err)
	}
	p.wallets = &testWallets{err: errs.New(errs.CodeInternal, "test.wallets")}
	if _, err := p.memberWallets(t.Context()); err == nil {
		t.Fatal("reader error = nil")
	}
	p.wallets = &testWallets{wallets: []port.MemberWallet{{UserID: user.ID, Address: user.Address}}}
	if got, err := p.memberWallets(t.Context()); err != nil || len(got) != 1 {
		t.Fatalf("memberWallets = %d, %v; want one wallet", len(got), err)
	}
	pool.Close()
	p.usdc = testkit.USDCMint
	p.reads = pool
	p.limit = rate.NewLimiter(rate.Inf, 1)
	if _, err := p.scan(t.Context(), port.MemberWallet{Address: user.Address}); err == nil {
		t.Fatal("scan cursor error = nil")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	p.limit = rate.NewLimiter(rate.Limit(1), 0)
	if _, err := p.scan(ctx, port.MemberWallet{}); err == nil {
		t.Fatal("scan limiter error = nil")
	}
	p.limit = rate.NewLimiter(rate.Inf, 1)
	if _, err := p.scan(t.Context(), port.MemberWallet{Address: "bad"}); err == nil {
		t.Fatal("scan ATA error = nil")
	}
}

func TestDepositPollerReportsWalletSchedulingErrors(t *testing.T) {
	t.Parallel()
	readerErr := errs.New(errs.CodeInternal, "test.wallets")
	p := &DepositPoller{wallets: &testWallets{err: readerErr}}
	if _, err := p.Tick(t.Context()); err == nil {
		t.Fatal("tick reader error = nil")
	}

	pool := testkit.DB(t)
	p = &DepositPoller{
		reads:   pool,
		wallets: &testWallets{wallets: []port.MemberWallet{{Address: "wallet"}}},
	}
	pool.Close()
	if _, err := p.memberWallets(t.Context()); err == nil {
		t.Fatal("cursor reader error = nil")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := p.scanWallets(ctx, nil); err == nil {
		t.Fatal("cancelled scanWallets error = nil")
	}
}

func TestDepositPollerPagesMemberWalletReader(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	first := make([]port.MemberWallet, port.MaxWalletPage)
	for i := range first {
		first[i] = port.MemberWallet{Address: "wallet"}
	}
	p := &DepositPoller{
		reads:   pool,
		wallets: &testWallets{pages: [][]port.MemberWallet{first, nil}},
	}
	if wallets, err := p.memberWallets(t.Context()); err != nil || len(wallets) != len(first) {
		t.Fatalf("memberWallets = %d, %v; want %d wallets", len(wallets), err, len(first))
	}
}

func TestDepositPollerAmountForRejectsOverflow(t *testing.T) {
	t.Parallel()
	p := DepositPoller{usdc: "usdc"}
	_, err := p.amountFor([]solana.Transfer{
		{Mint: chain.Mint{Address: "usdc", Decimals: 6}, Net: money.NewBaseUnits(math.MaxUint64, 6)},
		{Mint: chain.Mint{Address: "usdc", Decimals: 6}, Net: money.NewBaseUnits(1, 6)},
	})
	if err == nil {
		t.Fatal("amountFor overflow error = nil")
	}
}

func TestDepositPollerRejectsSlotsAboveInt64(t *testing.T) {
	t.Parallel()
	p := DepositPoller{}
	sig := solana.SignatureInfo{Slot: uint64(math.MaxInt64) + 1}
	if _, err := p.credit(t.Context(), port.MemberWallet{}, sig, money.MicrosFromUint64(1)); err == nil {
		t.Fatal("credit overflow error = nil")
	}
	if err := p.advance(t.Context(), "wallet", "signature", sig.Slot); err == nil {
		t.Fatal("advance overflow error = nil")
	}
}

func TestDepositPollerWrapsAdvanceFailures(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	pool.Close()
	p := NewDepositPoller(
		pool, db.New(pool, testkit.NewIDs(1), clock.Real{}), testkit.NewIDs(2), clock.Real{}, nil, nil,
		"usdc", time.Second, NewRPCLimiter(1), nil,
	)
	if err := p.advance(t.Context(), "wallet", "signature", 1); err == nil || errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("advance error = %v", err)
	}
}
