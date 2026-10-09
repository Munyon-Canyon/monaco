package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type watchWallets struct {
	wallets []port.MemberWallet
	pages   [][]port.MemberWallet
	called  int
	err     error
}

func (*watchWallets) MemberWallet(context.Context, ids.UserID) (port.MemberWallet, error) {
	return port.MemberWallet{}, nil
}

func (w *watchWallets) MemberWallets(context.Context, ids.UserID, int) ([]port.MemberWallet, error) {
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

type watchRPC struct {
	fixed    []solana.TokenAccountState
	reads    sqlc.DBTX
	pages    [][]solana.SignatureInfo
	sigErr   error
	slot     uint64
	tokenErr error
	accounts []solana.TokenAccountState
}

func (r *watchRPC) SignaturesFor(
	context.Context, chain.SolanaAddress, solana.SignaturesOpts,
) ([]solana.SignatureInfo, error) {
	if r.sigErr != nil {
		return nil, r.sigErr
	}
	if len(r.pages) == 0 {
		return nil, nil
	}
	page := r.pages[0]
	r.pages = r.pages[1:]
	return page, nil
}

func (r *watchRPC) Accounts(
	ctx context.Context, addrs []chain.SolanaAddress, _ uint64,
) (uint64, []solana.TokenAccountState, error) {
	if r.fixed != nil {
		return 0, r.fixed, nil
	}
	var slot uint64
	states := make([]solana.TokenAccountState, len(addrs))
	for i, addr := range addrs {
		var state, wallet, amountText, observedText string
		err := r.reads.QueryRow(ctx,
			`SELECT state, wallet_address, last_amount::text, observed_slot::text FROM deposit_watch_accounts
			WHERE token_account = $1`, addr).Scan(&state, &wallet, &amountText, &observedText)
		if err != nil {
			return 0, nil, err
		}
		observed, _ := strconv.ParseUint(observedText, 10, 64)
		amount, _ := strconv.ParseUint(amountText, 10, 64)
		slot = max(slot, observed)
		states[i] = solana.TokenAccountState{
			Address: addr, Exists: state == "open", Program: chain.SPLProgram, Mint: testkit.USDCMint,
			Owner: chain.SolanaAddress(wallet), State: "initialized", Amount: money.NewBaseUnits(amount, 6),
		}
	}
	return slot, states, nil
}

func (r *watchRPC) TokenAccounts(
	context.Context, chain.SolanaAddress, chain.Mint,
) (uint64, []solana.TokenAccountState, error) {
	return r.slot, r.accounts, r.tokenErr
}

func testTuning() DepositWatchTuning {
	return DepositWatchTuning{
		Rotation: 6 * time.Hour, RecoverySlots: 1000, Discovery: 6 * time.Hour,
		Spread: func(period time.Duration) time.Duration { return period / 2 },
	}
}

func testWatch(pool sqlc.DBTX, uow *db.UnitOfWork, rpc DepositWatchRPC, wallets port.WalletReader) *DepositWatch {
	if fake, ok := rpc.(*watchRPC); ok {
		fake.reads = pool
	}
	return NewDepositWatch(
		pool, uow, testkit.NewIDs(2), clock.Real{}, wallets, rpc, testkit.USDCMint, time.Second,
		rate.NewLimiter(rate.Inf, 0), 480, testTuning(), nil, zeroLedger{},
	)
}

func TestDepositWatchFetchesOnePageAndHonorsCancellation(t *testing.T) {
	t.Parallel()
	first := make([]solana.SignatureInfo, 1000)
	first[999].Signature = "before"
	p := &DepositWatch{rpc: &watchRPC{pages: [][]solana.SignatureInfo{first}}, limit: rate.NewLimiter(rate.Inf, 1)}
	opts := solana.SignaturesOpts{Before: "before", Until: "until", Limit: 1000}
	if got, err := p.signatures(t.Context(), "wallet", opts); err != nil || len(got) != 1000 {
		t.Fatalf("signatures = %d, %v", len(got), err)
	}
	p.limit = rate.NewLimiter(rate.Limit(1), 0)
	if _, err := p.signatures(t.Context(), "wallet", opts); !errors.Is(err, errRateBudgetSpent) {
		t.Fatalf("signatures with a spent budget = %v, want errRateBudgetSpent", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := p.signatures(ctx, "wallet", opts); err == nil || errors.Is(err, errRateBudgetSpent) {
		t.Fatalf("cancelled signatures error = %v, want a failure", err)
	}
	p.rpc = &watchRPC{sigErr: errs.New(errs.CodeInternal, "test.rpc")}
	p.limit = rate.NewLimiter(rate.Inf, 0)
	if _, err := p.signatures(t.Context(), "wallet", opts); errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("signatures rpc error = %v, want %s", err, errs.CodeRPCUnavailable)
	}
}

func TestDepositWatchReportsItsNameAndInterval(t *testing.T) {
	t.Parallel()
	p := NewDepositWatch(
		nil, nil, nil, nil, nil, nil, "usdc", time.Second, nil, 0, DepositWatchTuning{}, nil, zeroLedger{},
	)
	if p.Interval() != time.Second || p.Name() != "funding.deposit_watch" {
		t.Fatalf("name/interval = %q/%s", p.Name(), p.Interval())
	}
}

func TestDepositWatchFirstSightReadsKnownWalletsOncePerPage(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	other := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	for _, u := range []testkit.SeededUser{user, other} {
		if _, err := pool.Exec(t.Context(),
			`INSERT INTO deposit_watch_wallets (wallet_address, user_id, first_seen_slot, first_seen_at,
				discovery_due_at) VALUES ($1, $2, 0, now(), now() + interval '1 day')`, u.Address, u.ID.UUID()); err != nil {
			t.Fatal(err)
		}
	}
	page := []port.MemberWallet{{UserID: user.ID, Address: user.Address}, {UserID: other.ID, Address: other.Address}}
	p := testWatch(pool, nil, &watchRPC{}, nil)
	testkit.AssertQueries(t, "DepositWatch seedUnknown", func() {
		if n, err := p.seedUnknown(t.Context(), page); err != nil || n != 0 {
			t.Fatalf("seedUnknown = %d, %v; want every wallet known", n, err)
		}
	})
}

func TestDepositWatchFirstSightReportsAnUnreadableDatabase(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	pool.Close()
	first := make([]port.MemberWallet, port.MaxWalletPage)
	p := testWatch(pool, nil, &watchRPC{}, &watchWallets{pages: [][]port.MemberWallet{first, nil}})
	if _, err := p.firstSight(t.Context()); err == nil {
		t.Fatal("firstSight with an unreadable database error = nil")
	}
}

func TestDepositWatchFirstSightPagesTheMemberWalletReader(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	known := make([]port.MemberWallet, port.MaxWalletPage)
	for i := range known {
		known[i] = port.MemberWallet{Address: chain.SolanaAddress("w")}
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO deposit_watch_wallets (wallet_address, user_id, first_seen_slot,
		first_seen_at, discovery_due_at) VALUES ('w', $1, 0, now(), now() + interval '1 day')`, testkit.SeedUser(t, pool, testkit.UserOpts{}).ID.UUID()); err != nil {
		t.Fatal(err)
	}
	p := testWatch(pool, nil, &watchRPC{}, &watchWallets{pages: [][]port.MemberWallet{known, nil}})
	if n, err := p.firstSight(t.Context()); err != nil || n != 0 {
		t.Fatalf("firstSight = %d, %v; want two reader pages and no wallet seeded", n, err)
	}
	p = testWatch(pool, nil, &watchRPC{}, &watchWallets{err: errs.New(errs.CodeInternal, "test.wallets")})
	if _, err := p.Tick(t.Context()); err == nil {
		t.Fatal("Tick with a failing wallet reader error = nil")
	}
}

func TestDepositWatchReportsAnUnreadableDirtyList(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	pool.Close()
	p := testWatch(pool, nil, &watchRPC{}, &watchWallets{})
	if _, _, err := p.catchUpDirty(t.Context()); err == nil {
		t.Fatal("catchUpDirty error = nil")
	}
}

func TestDepositWatchRejectsSlotsAboveInt64(t *testing.T) {
	t.Parallel()
	sig := solana.SignatureInfo{Slot: uint64(math.MaxInt64) + 1}
	row := sqlc.DepositWatchDirtyAccountsRow{}
	if _, err := watchCandidates(row, []solana.SignatureInfo{sig}); err == nil {
		t.Fatal("watchCandidates overflow error = nil")
	}
	if _, err := watchCandidates(row, []solana.SignatureInfo{{Slot: 1}, {Slot: sig.Slot, Failed: true}}); err == nil {
		t.Fatal("watchCandidates failed signature overflow error = nil")
	}
	p := testWatch(nil, nil, &watchRPC{slot: sig.Slot}, nil)
	if err := p.seedWallet(t.Context(), port.MemberWallet{Address: testkit.USDCMint}); err == nil {
		t.Fatal("seedWallet slot overflow error = nil")
	}
}

func TestDepositWatchReportsSeedFailures(t *testing.T) {
	t.Parallel()
	wallet := port.MemberWallet{Address: testkit.USDCMint}
	p := testWatch(nil, nil, &watchRPC{tokenErr: errs.New(errs.CodeInternal, "test.tokens")}, nil)
	if err := p.seedWallet(t.Context(), wallet); errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("seedWallet token accounts error = %v, want %s", err, errs.CodeRPCUnavailable)
	}
	p = testWatch(nil, nil, &watchRPC{}, nil)
	if err := p.seedWallet(
		t.Context(),
		port.MemberWallet{Address: "bad"},
	); errs.CodeOf(
		err,
	) != errs.CodeInvalidAddress {
		t.Fatalf("seedWallet invalid address error = %v, want %s", err, errs.CodeInvalidAddress)
	}
	p = testWatch(nil, nil, &watchRPC{sigErr: errs.New(errs.CodeInternal, "test.sigs")}, nil)
	if err := p.seedWallet(t.Context(), wallet); errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("seedWallet signatures error = %v, want %s", err, errs.CodeRPCUnavailable)
	}
	p.limit = rate.NewLimiter(rate.Limit(1), 0)
	if err := p.seedWallet(t.Context(), wallet); !errors.Is(err, errRateBudgetSpent) {
		t.Fatalf("seedWallet with a spent budget = %v, want errRateBudgetSpent", err)
	}
}

func TestDepositWatchFirstSightSkipsSignaturesAboveTheContextSlotAcrossPages(t *testing.T) {
	t.Parallel()
	above := make([]solana.SignatureInfo, depositSignaturePageSize)
	for i := range above {
		above[i] = solana.SignatureInfo{Signature: "above", Slot: 9}
	}
	rpc := &watchRPC{
		pages: [][]solana.SignatureInfo{above, {{Signature: "at", Slot: 5}, {Signature: "older", Slot: 4}}},
	}
	got, err := testWatch(nil, nil, rpc, nil).scanFromTip(t.Context(), "account", 5, time.Time{})
	if err != nil || got.high.Signature != "at" || len(got.window) != 2 || got.resume != "" {
		t.Fatalf("scanFromTip = %+v, %v; want the newest signature at or below slot 5 and no resume", got, err)
	}
	rpc = &watchRPC{pages: [][]solana.SignatureInfo{{{Signature: "above", Slot: 9}}}}
	if got, err := testWatch(nil, nil, rpc, nil).scanFromTip(t.Context(), "account", 5, time.Time{}); err != nil ||
		got.high.Signature != "" || len(got.window) != 0 {
		t.Fatalf("scanFromTip = %+v, %v; want none", got, err)
	}
}

func TestDepositWatchFirstSightStopsAtTheCreationFloorAndResumesPastAFullPage(t *testing.T) {
	t.Parallel()
	floor := clock.Real{}.Now().Add(-time.Hour)
	inside, before := floor.Add(time.Minute), floor.Add(-time.Minute)
	rpc := &watchRPC{pages: [][]solana.SignatureInfo{{
		{Signature: "new", Slot: 5, BlockTime: inside},
		{Signature: "edge", Slot: 4, BlockTime: floor},
		{Signature: "old", Slot: 3, BlockTime: before},
		{Signature: "older", Slot: 2, BlockTime: before},
	}}}
	got, err := testWatch(nil, nil, rpc, nil).scanFromTip(t.Context(), "account", 5, floor)
	if err != nil || got.high.Signature != "new" || len(got.window) != 2 || got.resume != "" {
		t.Fatalf("scanFromTip = %+v, %v; want new and edge in the window, none resumed", got, err)
	}
	full := make([]solana.SignatureInfo, depositSignaturePageSize)
	for i := range full {
		full[i] = solana.SignatureInfo{Signature: chain.Signature(fmt.Sprintf("s%d", i)), Slot: 5, BlockTime: inside}
	}
	rpc = &watchRPC{pages: [][]solana.SignatureInfo{full}}
	got, err = testWatch(nil, nil, rpc, nil).scanFromTip(t.Context(), "account", 5, floor)
	if err != nil || got.high.Signature != "s0" || len(got.window) != len(full) || got.resume != "s999" {
		t.Fatalf("scanFromTip = %d in window, resume %q, %v; want a full window resuming at s999",
			len(got.window), got.resume, err)
	}
	rpc = &watchRPC{sigErr: errs.New(errs.CodeInternal, "test.down")}
	if _, err := testWatch(nil, nil, rpc, nil).scanFromTip(t.Context(), "account", 5, floor); err == nil {
		t.Fatal("scanFromTip with a failing RPC = nil")
	}
}

func TestDepositWatchResumedFirstSightFinishesFromTheCheckpoint(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	floor := clock.Real{}.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	canonical, err := chain.AssociatedTokenAccount(user.Address, testkit.USDCMint, chain.SPLProgram)
	if err != nil {
		t.Fatal(err)
	}
	seed := watchSeed{
		state: solana.TokenAccountState{Address: canonical, Exists: true},
		tipScan: tipScan{
			high:   solana.SignatureInfo{Signature: "tip", Slot: 7},
			resume: "cursor",
		},
	}
	rpc := &watchRPC{pages: [][]solana.SignatureInfo{{
		{Signature: "inside", Slot: 6, BlockTime: floor.Add(time.Minute)},
		{Signature: "outside", Slot: 5, BlockTime: floor.Add(-time.Minute)},
	}}}
	p := testWatch(pool, db.New(pool, testkit.NewIDs(1), clock.Real{}), rpc, nil)
	wallet := port.MemberWallet{UserID: user.ID, Address: user.Address}
	plan := seedPlan{canonical: canonical, seeds: []watchSeed{seed}, slot: 7, floor: floor}
	if err := p.persistSeeds(t.Context(), wallet, plan); err != nil {
		t.Fatal(err)
	}
	assertResumeCheckpoint(t, pool)
	scanned, recorded, err := p.catchUpDirty(observability.WithActor(t.Context(), "system:test"))
	if err != nil || scanned != 1 || recorded != 1 {
		t.Fatalf("catchUpDirty = %d/%d, %v; want 1/1", scanned, recorded, err)
	}
	assertResumeCompleted(t, pool)
}

func assertResumeCheckpoint(t *testing.T, pool sqlc.DBTX) {
	t.Helper()
	var before, top string
	var opening *string
	if err := pool.QueryRow(t.Context(), `SELECT page_before, page_top_signature, opening_micros::text
		FROM deposit_watch_accounts a JOIN deposit_watch_wallets w USING (wallet_address)`).
		Scan(&before, &top, &opening); err != nil || before != "cursor" || top != "tip" || opening != nil {
		t.Fatalf("checkpoint = %q/%q/%v, %v; want cursor/tip/NULL", before, top, opening, err)
	}
}

func assertResumeCompleted(t *testing.T, pool sqlc.DBTX) {
	t.Helper()
	var high string
	var pending *string
	var clean, dirty int64
	var floor *time.Time
	if err := pool.QueryRow(t.Context(),
		`SELECT high_signature, page_before, history_floor, clean_gen, dirty_gen FROM deposit_watch_accounts`).
		Scan(&high, &pending, &floor, &clean, &dirty); err != nil ||
		high != "tip" || pending != nil || floor != nil || clean != dirty {
		t.Fatalf("after catch-up = %q/%v/%v/%d/%d, %v; want tip, cleared, clean",
			high, pending, floor, clean, dirty, err)
	}
}

func watchWithDroppedTable(t *testing.T, table string) (*DepositWatch, context.Context, testkit.SeededUser) {
	t.Helper()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	if _, err := pool.Exec(t.Context(), `DROP TABLE `+table+` CASCADE`); err != nil {
		t.Fatal(err)
	}
	p := testWatch(pool, db.New(pool, testkit.NewIDs(91), clock.Real{}), &watchRPC{}, nil)
	return p, observability.WithActor(t.Context(), "system:test"), user
}

func TestDepositWatchReturnsCommitFailuresWithoutAccountRows(t *testing.T) {
	t.Parallel()
	p, ctx, user := watchWithDroppedTable(t, "deposit_watch_accounts")
	row := sqlc.DepositWatchDirtyAccountsRow{
		TokenAccount: "account", WalletAddress: string(user.Address), UserID: user.ID.UUID(),
	}
	short := []solana.SignatureInfo{{Signature: "signature", Slot: 1, Failed: true}}
	for name, page := range map[string][]solana.SignatureInfo{
		"checkpoint": make([]solana.SignatureInfo, depositSignaturePageSize), "complete": nil, "short": short,
	} {
		if _, err := p.commitPage(ctx, row, page); err == nil {
			t.Fatalf("commitPage %s error = nil", name)
		}
	}
	wallet := port.MemberWallet{UserID: user.ID, Address: user.Address}
	seeds := []watchSeed{{state: solana.TokenAccountState{Address: "account"}}}
	if err := p.persistSeeds(
		ctx,
		wallet,
		seedPlan{canonical: "other", seeds: seeds, slot: 1, opening: "0"},
	); err == nil {
		t.Fatal("persistSeeds account error = nil")
	}
}

func TestDepositWatchReturnsSeedFailuresWithoutWalletRows(t *testing.T) {
	t.Parallel()
	p, ctx, user := watchWithDroppedTable(t, "deposit_watch_wallets")
	wallet := port.MemberWallet{UserID: user.ID, Address: user.Address}
	if err := p.persistSeeds(ctx, wallet, seedPlan{canonical: "other", slot: 1, opening: "0"}); err == nil {
		t.Fatal("persistSeeds wallet error = nil")
	}
}

func TestDepositWatchReturnsCandidateFailuresFromFirstSight(t *testing.T) {
	t.Parallel()
	p, ctx, user := watchWithDroppedTable(t, "deposit_candidates")
	wallet := port.MemberWallet{UserID: user.ID, Address: user.Address}
	plan := seedPlan{candidates: []DepositCandidate{{
		Signature: "sig", Wallet: user.Address, UserID: user.ID, Slot: 1, Source: depositCandidateSourcePoller,
	}}}
	if err := p.persistSeeds(ctx, wallet, plan); err == nil {
		t.Fatal("persistSeeds candidate error = nil")
	}
}

func seedWatchRows(t *testing.T, pool sqlc.DBTX, user testkit.SeededUser, high string) {
	t.Helper()
	ata, err := chain.AssociatedTokenAccount(user.Address, testkit.USDCMint, chain.SPLProgram)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO deposit_watch_wallets (wallet_address, user_id, first_seen_slot, first_seen_at, discovery_due_at)
		VALUES ($1, $2, 0, now(), now() + interval '1 day')`, user.Address, user.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO deposit_watch_accounts (token_account, wallet_address, canonical, state, dirty_gen,
			high_signature, recovery_due_at) VALUES ($1, $2, true, 'open', 1, NULLIF($3, ''), now() + interval '1 day')`,
		ata, user.Address, high); err != nil {
		t.Fatal(err)
	}
}

func TestDepositWatchCatchesUpADirtyAccountPageByPage(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	seedWatchRows(t, pool, user, "baseline")
	full := make([]solana.SignatureInfo, depositSignaturePageSize)
	for i := range full {
		full[i] = solana.SignatureInfo{Signature: chain.Signature(fmt.Sprintf("sig%d", 2000-i)), Slot: uint64(2000 - i)}
	}
	rpc := &watchRPC{pages: [][]solana.SignatureInfo{full, {{Signature: "last", Slot: 5}}}}
	p := testWatch(pool, db.New(pool, testkit.NewIDs(1), clock.Real{}), rpc, &watchWallets{})
	report, err := p.Tick(observability.WithActor(t.Context(), "system:test"))
	if err != nil || report.Changed != 1001 || report.Scanned != 2 {
		t.Fatalf("Tick = %+v, %v; want 1001 candidates from one dirty account (one gated, one scanned)", report, err)
	}
	var high string
	var page *string
	var clean, dirty int64
	if err := pool.QueryRow(t.Context(),
		`SELECT high_signature, page_before, clean_gen, dirty_gen FROM deposit_watch_accounts`,
	).Scan(&high, &page, &clean, &dirty); err != nil || high != "sig2000" || page != nil || clean != dirty {
		t.Fatalf("high/page/clean/dirty = %q/%v/%d/%d, %v; want sig2000, no page, clean", high, page, clean, dirty, err)
	}
}

func TestDepositWatchFirstSightSeedsTheCanonicalAccountAndEveryReturnedOne(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	other := chain.SolanaAddress("Other111111111111111111111111111111111111111")
	rpc := &watchRPC{slot: 9, accounts: []solana.TokenAccountState{{Address: other, Exists: true}}}
	rpc.pages = [][]solana.SignatureInfo{{{Signature: "above", Slot: 12}, {Signature: "tip", Slot: 9}}, nil}
	wallets := &watchWallets{wallets: []port.MemberWallet{{UserID: user.ID, Address: user.Address}}}
	p := testWatch(pool, db.New(pool, testkit.NewIDs(1), clock.Real{}), rpc, wallets)
	report, err := p.Tick(observability.WithActor(t.Context(), "system:test"))
	if err != nil || report.Scanned != 1 || report.Changed != 0 {
		t.Fatalf("Tick = %+v, %v; want one wallet seeded", report, err)
	}
	var accounts, withTip int
	var firstSeen int64
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*), count(*) FILTER (WHERE high_signature = 'tip'), min(w.first_seen_slot)
		FROM deposit_watch_accounts a JOIN deposit_watch_wallets w USING (wallet_address)`,
	).Scan(&accounts, &withTip, &firstSeen); err != nil || accounts != 2 || withTip != 1 || firstSeen != 9 {
		t.Fatalf("accounts/withTip/firstSeen = %d/%d/%d, %v; want 2/1/9", accounts, withTip, firstSeen, err)
	}
}

func TestDepositWatchTickEndsQuietlyWhenTheRateBudgetIsSpent(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	wallets := &watchWallets{wallets: []port.MemberWallet{{UserID: user.ID, Address: user.Address}}}
	p := testWatch(pool, db.New(pool, testkit.NewIDs(1), clock.Real{}), &watchRPC{}, wallets)
	p.limit = rate.NewLimiter(rate.Limit(1), 0)
	if report, err := p.Tick(t.Context()); err != nil || report.Scanned != 0 {
		t.Fatalf("Tick = %+v, %v; want no error and nothing scanned", report, err)
	}
}

func TestDepositWatchStopsTheTickWhenADirtyAccountCannotBeScanned(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	seedWatchRows(t, pool, user, "baseline")
	rpc := &watchRPC{sigErr: errs.New(errs.CodeInternal, "test.sigs")}
	p := testWatch(pool, db.New(pool, testkit.NewIDs(1), clock.Real{}), rpc, &watchWallets{})
	if _, err := p.Tick(t.Context()); errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("Tick = %v, want %s", err, errs.CodeRPCUnavailable)
	}
}

func TestDepositWatchRejectsAPageWithASlotAboveInt64(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	seedWatchRows(t, pool, user, "baseline")
	rpc := &watchRPC{pages: [][]solana.SignatureInfo{{{Signature: "huge", Slot: math.MaxInt64 + 1}}}}
	p := testWatch(pool, db.New(pool, testkit.NewIDs(1), clock.Real{}), rpc, &watchWallets{})
	if _, err := p.Tick(t.Context()); err == nil {
		t.Fatal("Tick error = nil")
	}
}

func TestDepositWatchReturnsACommitFailure(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	if _, err := pool.Exec(t.Context(), `DROP TABLE deposit_watch_accounts CASCADE`); err != nil {
		t.Fatal(err)
	}
	p := testWatch(pool, db.New(pool, testkit.NewIDs(1), clock.Real{}), &watchRPC{}, nil)
	row := sqlc.DepositWatchDirtyAccountsRow{TokenAccount: "account", WalletAddress: string(user.Address)}
	page := []solana.SignatureInfo{{Signature: "signature", Slot: 1, Failed: true}}
	ctx := observability.WithActor(t.Context(), "system:test")
	if _, err := p.commitPage(ctx, row, page); err == nil {
		t.Fatal("commitPage error = nil")
	}
	recorded := []solana.SignatureInfo{{Signature: "signature", Slot: 1}}
	if _, err := p.commitPage(t.Context(), row, recorded); err == nil {
		t.Fatal("commitPage without an actor error = nil")
	}
}

type zeroLedger struct{}

func (zeroLedger) WalletLedgerMicros(
	context.Context, ids.UserID, chain.SolanaAddress,
) (money.SignedMicros, int, error) {
	return money.SignedMicros{}, 0, nil
}
