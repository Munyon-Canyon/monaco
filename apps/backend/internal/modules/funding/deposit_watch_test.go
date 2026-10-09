package funding_test

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
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

type watchRPC struct {
	signatures    []solana.SignatureInfo
	signErr       error
	signaturesFor func(chain.Signature, chain.Signature, int) []solana.SignatureInfo
	signCalls     int
	limits        []int
	minSlots      []uint64
	untils        []chain.Signature
	slot          uint64
	accounts      []solana.TokenAccountState

	pool          *pgxpool.Pool
	observe       func([]chain.SolanaAddress) (uint64, []solana.TokenAccountState)
	accountsCalls int
	accountsErr   error
}

func (r *watchRPC) Accounts(
	ctx context.Context, addrs []chain.SolanaAddress, _ uint64,
) (uint64, []solana.TokenAccountState, error) {
	r.accountsCalls++
	if r.accountsErr != nil {
		return 0, nil, r.accountsErr
	}
	if r.observe != nil {
		slot, states := r.observe(addrs)
		return slot, states, nil
	}
	var slot uint64
	states := make([]solana.TokenAccountState, len(addrs))
	for i, addr := range addrs {
		var state, wallet, amountText, observedText string
		err := r.pool.QueryRow(ctx,
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

func (r *watchRPC) SignaturesFor(
	_ context.Context, _ chain.SolanaAddress, opts solana.SignaturesOpts,
) ([]solana.SignatureInfo, error) {
	r.signCalls++
	r.limits = append(r.limits, opts.Limit)
	r.minSlots = append(r.minSlots, opts.MinContextSlot)
	r.untils = append(r.untils, opts.Until)
	if r.signErr != nil {
		return nil, r.signErr
	}
	if r.signaturesFor != nil {
		return r.signaturesFor(opts.Before, opts.Until, opts.Limit), nil
	}
	return r.signatures, nil
}

func (r *watchRPC) TokenAccounts(
	context.Context, chain.SolanaAddress, chain.Mint,
) (uint64, []solana.TokenAccountState, error) {
	return r.slot, r.accounts, nil
}

func watchTuning() app.DepositWatchTuning {
	return app.DepositWatchTuning{
		Rotation: 6 * time.Hour, RecoverySlots: 1000, Discovery: 6 * time.Hour,
		Spread: func(period time.Duration) time.Duration { return period / 2 },
	}
}

func watchActor(t *testing.T) context.Context {
	t.Helper()
	return observability.WithActor(t.Context(), "system:poller.funding.deposit_watch")
}

func canonicalAccount(t *testing.T, wallet chain.SolanaAddress) chain.SolanaAddress {
	t.Helper()
	ata, err := chain.AssociatedTokenAccount(wallet, testkit.USDCMint, chain.SPLProgram)
	if err != nil {
		t.Fatal(err)
	}
	return ata
}

func seedWatchAccount(t *testing.T, pool *pgxpool.Pool, user testkit.SeededUser) chain.SolanaAddress {
	t.Helper()
	ata := canonicalAccount(t, user.Address)
	if _, err := pool.Exec(
		t.Context(),
		`INSERT INTO deposit_watch_wallets (wallet_address, user_id, first_seen_slot, first_seen_at, discovery_due_at)
		VALUES ($1, $2, 0, now(), now() + interval '1 day')`, user.Address, user.ID.UUID(),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(
		t.Context(),
		`INSERT INTO deposit_watch_accounts (
			token_account, wallet_address, canonical, state, dirty_gen, high_signature, recovery_due_at)
		VALUES ($1, $2, true, 'open', 1, 'baseline', now() + interval '1 day')`, ata, user.Address,
	); err != nil {
		t.Fatal(err)
	}
	return ata
}

func markDirty(t *testing.T, pool *pgxpool.Pool, account chain.SolanaAddress, slot int64) {
	t.Helper()
	n, err := sqlc.New(pool).MarkDepositWatchAccountDirty(
		t.Context(), sqlc.MarkDepositWatchAccountDirtyParams{TokenAccount: string(account), Slot: slot},
	)
	if err != nil || n != 1 {
		t.Fatalf("mark dirty = %d, %v; want 1 row", n, err)
	}
}

type accountRow struct {
	High, PageBefore, PageTop string
	HighSlot                  int64
	DirtyGen, CleanGen        int64
	ScannedAt                 *time.Time
}

func loadAccount(t *testing.T, pool *pgxpool.Pool, account chain.SolanaAddress) accountRow {
	t.Helper()
	var r accountRow
	err := pool.QueryRow(
		t.Context(),
		`SELECT coalesce(high_signature, ''), coalesce(page_before, ''), coalesce(page_top_signature, ''),
		high_slot, dirty_gen, clean_gen, scanned_at FROM deposit_watch_accounts WHERE token_account = $1`, account,
	).Scan(&r.High, &r.PageBefore, &r.PageTop, &r.HighSlot, &r.DirtyGen, &r.CleanGen, &r.ScannedAt)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func newDepositWatch(
	t *testing.T, pool *pgxpool.Pool, user testkit.SeededUser, now time.Time, rpc *watchRPC, uowID, watchID uint64,
) *app.DepositWatch {
	t.Helper()
	seedWatchAccount(t, pool, user)
	return watchFor(pool, user, now, rpc, unlimited(), uowID, watchID)
}

func watchFor(
	pool *pgxpool.Pool, user testkit.SeededUser, now time.Time, rpc *watchRPC, limit app.RPCLimiter,
	uowID, watchID uint64,
) *app.DepositWatch {
	return watchForWallet(
		pool,
		identity.MemberWallet{UserID: user.ID, Address: user.Address},
		now,
		rpc,
		limit,
		uowID,
		watchID,
	)
}

func watchForWallet(
	pool *pgxpool.Pool, wallet identity.MemberWallet, now time.Time, rpc *watchRPC, limit app.RPCLimiter,
	uowID, watchID uint64,
) *app.DepositWatch {
	rpc.pool = pool
	return app.NewDepositWatch(
		pool,
		db.New(pool, testkit.NewIDs(uowID), testkit.NewClock(now)),
		testkit.NewIDs(watchID),
		testkit.NewClock(now),
		fakes.NewIdentity(nil, []identity.MemberWallet{wallet}),
		rpc,
		testkit.USDCMint,
		app.DepositPollInterval,
		limit,
		480,
		watchTuning(),
		noop.Int64Counter{}, &stubLedger{},
	)
}

func TestDepositWatchStartsNewWalletsAtTheChainTip(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	old := solana.SignatureInfo{Signature: "old", Slot: 1, BlockTime: depositBlockTime()}
	later := solana.SignatureInfo{Signature: "later", Slot: 2, BlockTime: depositBlockTime()}
	rpc := watchRPC{
		slot: 1,
		signaturesFor: func(_ chain.Signature, until chain.Signature, _ int) []solana.SignatureInfo {
			if until == "old" {
				return []solana.SignatureInfo{later}
			}
			return []solana.SignatureInfo{old}
		},
	}
	p := watchFor(pool, user, now, &rpc, unlimited(), 70, 71)
	ctx := watchActor(t)
	report, err := p.Tick(ctx)
	if err != nil || report.Changed != 0 || report.Scanned != 1 {
		t.Fatalf("first Tick = %+v, %v; want one wallet seeded and no candidates", report, err)
	}
	markDirty(t, pool, canonicalAccount(t, user.Address), 2)
	if report, err := p.Tick(ctx); err != nil || report.Changed != 1 {
		t.Fatalf("second Tick = %+v, %v", report, err)
	}
	if got, want := fmt.Sprint(rpc.limits), "[1000 1000]"; got != want {
		t.Fatalf("signature limits = %s, want %s", got, want)
	}
	if got, want := fmt.Sprint(rpc.minSlots), "[1 2]"; got != want {
		t.Fatalf("min context slots = %s, want %s", got, want)
	}
	assertCandidateSignatures(ctx, t, pool, user.Address, "later")
	var firstSeen int64
	var state string
	if err := pool.QueryRow(ctx, `SELECT w.first_seen_slot, a.state FROM deposit_watch_wallets w
		JOIN deposit_watch_accounts a ON a.wallet_address = w.wallet_address`).Scan(&firstSeen, &state); err != nil ||
		firstSeen != 1 || state != "missing" {
		t.Fatalf("first_seen_slot/state = %d/%q, %v; want 1/missing", firstSeen, state, err)
	}
}

func TestDepositWatchFirstSightTracksEveryUSDCAccountTheWalletOwns(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	ata := canonicalAccount(t, user.Address)
	const other chain.SolanaAddress = "Other111111111111111111111111111111111111111"
	rpc := watchRPC{
		slot: 5,
		accounts: []solana.TokenAccountState{
			{Address: ata, Exists: true, Amount: money.NewBaseUnits(7, 6)},
			{Address: other, Exists: true, Amount: money.NewBaseUnits(3, 6)},
		},
		signaturesFor: func(_ chain.Signature, _ chain.Signature, _ int) []solana.SignatureInfo {
			return []solana.SignatureInfo{{Signature: "above", Slot: 9}, {Signature: "tip", Slot: 5}}
		},
	}
	if _, err := watchFor(pool, user, now, &rpc, unlimited(), 80, 81).Tick(watchActor(t)); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(t.Context(), `SELECT token_account, canonical, state, last_amount::text, high_signature,
		high_slot, observed_slot, dirty_gen FROM deposit_watch_accounts ORDER BY canonical DESC`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var account, state, amount, high string
		var canonical bool
		var highSlot, observed, dirty int64
		if err := rows.Scan(&account, &canonical, &state, &amount, &high, &highSlot, &observed, &dirty); err != nil {
			t.Fatal(err)
		}
		got = append(got, fmt.Sprintf("%t %t %s %s %s %d %d %d",
			account == string(ata), canonical, state, amount, high, highSlot, observed, dirty))
	}
	want := []string{"true true open 7 tip 5 5 0", "false false open 3 tip 5 5 0"}
	if rows.Err() != nil || !slices.Equal(got, want) {
		t.Fatalf("accounts = %v, %v; want %v", got, rows.Err(), want)
	}
	assertCandidateSignatures(t.Context(), t, pool, user.Address)
}

func wholePageHistory() []solana.SignatureInfo {
	history := make([]solana.SignatureInfo, 0, 1000)
	for i := 1000; i > 0; i-- {
		history = append(history, solana.SignatureInfo{
			Signature: chain.Signature(fmt.Sprintf("sig%d", i)), Slot: uint64(i), Failed: i <= 997,
		})
	}
	return history
}

func assertWholePageCheckpoint(t *testing.T, got accountRow, scannedAt time.Time) {
	t.Helper()
	if got.PageBefore != "sig1" || got.PageTop != "sig1000" || got.High != "baseline" || got.CleanGen != 0 {
		t.Fatalf("checkpoint = %+v, want before sig1, top sig1000, high unchanged, still dirty", got)
	}
	if got.ScannedAt == nil || !got.ScannedAt.Equal(scannedAt) {
		t.Fatalf("scanned_at = %v, want %s", got.ScannedAt, scannedAt)
	}
}

func TestDepositWatchCommitsCandidatesWithTheCheckpointOfAWholePage(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	ata := seedWatchAccount(t, pool, user)
	history := wholePageHistory()
	rpc := watchRPC{signaturesFor: func(before, until chain.Signature, limit int) []solana.SignatureInfo {
		return signaturesFromHistory(history, before, until, limit)
	}}
	watchClock := testkit.NewClock(now)
	budget := &rpcBudget{}
	rpc.pool = pool
	p := app.NewDepositWatch(
		pool, db.New(pool, testkit.NewIDs(76), watchClock), testkit.NewIDs(77), watchClock,
		fakes.NewIdentity(nil, []identity.MemberWallet{{UserID: user.ID, Address: user.Address}}),
		&rpc, testkit.USDCMint, app.DepositPollInterval, budget, 480, watchTuning(), noop.Int64Counter{}, &stubLedger{},
	)
	ctx := watchActor(t)
	watchClock.Advance(time.Second)
	budget.refill(2)
	if report, err := p.Tick(ctx); err != nil || report.Changed != 3 {
		t.Fatalf("first Tick = %+v, %v; want three candidates", report, err)
	}
	assertWholePageCheckpoint(t, loadAccount(t, pool, ata), now.Add(time.Second))
	assertCandidateSignatures(ctx, t, pool, user.Address, "sig1000", "sig999", "sig998")
	budget.refill(2)
	if report, err := p.Tick(ctx); err != nil || report.Changed != 0 {
		t.Fatalf("second Tick = %+v, %v; want the walk finished", report, err)
	}
	got := loadAccount(t, pool, ata)
	if got.High != "sig1000" || got.HighSlot != 1000 || got.PageBefore != "" || got.PageTop != "" ||
		got.CleanGen != got.DirtyGen {
		t.Fatalf("finished account = %+v, want high sig1000 at slot 1000, page cleared, clean", got)
	}
}

func TestDepositWatchFixtureI5_LaggingNodeLeavesTheAccountDirtyAndNothingAdvances(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	ata := seedWatchAccount(t, pool, user)
	markDirty(t, pool, ata, 77)
	before := loadAccount(t, pool, ata)
	rpc := watchRPC{signErr: errs.New(errs.CodeRPCUnavailable, "test.minContextSlotNotReached")}
	report, err := watchFor(pool, user, now, &rpc, unlimited(), 33, 34).Tick(watchActor(t))
	if errs.CodeOf(err) != errs.CodeRPCUnavailable || report.Changed != 0 {
		t.Fatalf("Tick = %+v, %v; want %s", report, err, errs.CodeRPCUnavailable)
	}
	if got := loadAccount(t, pool, ata); got != before || got.CleanGen >= got.DirtyGen {
		t.Fatalf("account = %+v, want unchanged %+v and still dirty", got, before)
	}
	if got := fmt.Sprint(rpc.minSlots); got != "[77]" {
		t.Fatalf("min context slots = %s, want [77]: the scan must wait for the hinted slot", got)
	}
	assertCandidateSignatures(t.Context(), t, pool, user.Address)
}

func TestDepositWatchFixtureI7_TwoSignaturesInOneSlotSplitAcrossAPageBoundaryAreBothRecorded(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	ata := seedWatchAccount(t, pool, user)
	history := make([]solana.SignatureInfo, 0, 1001)
	for i := 1001; i > 0; i-- {
		history = append(history, solana.SignatureInfo{
			Signature: chain.Signature(fmt.Sprintf("sig%d", i)), Slot: uint64(max(i, 2)), Failed: i > 2,
		})
	}
	rpc := watchRPC{signaturesFor: func(before, until chain.Signature, limit int) []solana.SignatureInfo {
		return signaturesFromHistory(history, before, until, limit)
	}}
	p := watchFor(pool, user, now, &rpc, unlimited(), 35, 36)
	ctx := watchActor(t)
	if report, err := p.Tick(ctx); err != nil || report.Changed != 2 || rpc.signCalls != 2 {
		t.Fatalf("Tick = %+v, %v with %d calls; want two candidates over two pages", report, err, rpc.signCalls)
	}
	assertCandidateSignatures(ctx, t, pool, user.Address, "sig1", "sig2")
	if got := loadAccount(t, pool, ata); got.High != "sig1001" || got.CleanGen != got.DirtyGen {
		t.Fatalf("account = %+v, want high sig1001 and clean", got)
	}
}

func TestDepositWatchFixtureI9_ClosedAndReopenedAccountKeepsItsCursor(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	ata := seedWatchAccount(t, pool, user)
	rpc := watchRPC{signatures: []solana.SignatureInfo{{Signature: "first", Slot: 3, BlockTime: depositBlockTime()}}}
	p := watchFor(pool, user, now, &rpc, unlimited(), 37, 38)
	ctx := watchActor(t)
	if _, err := p.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"closed", "open"} {
		const setState = `UPDATE deposit_watch_accounts SET state = $2 WHERE token_account = $1`
		if _, err := pool.Exec(ctx, setState, ata, state); err != nil {
			t.Fatal(err)
		}
		markDirty(t, pool, ata, 9)
		rpc.signatures = nil
		if _, err := p.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if got := fmt.Sprint(rpc.untils); got != "[baseline first first]" {
		t.Fatalf("until marks = %s, want each scan to stop at the cursor the row kept", got)
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM deposit_watch_accounts`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("account rows = %d, %v; want the one row to survive", rows, err)
	}
}

func TestDepositWatchGuardsTheGenerationAgainstAHintDuringTheScan(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	ata := seedWatchAccount(t, pool, user)
	rpc := watchRPC{signaturesFor: func(chain.Signature, chain.Signature, int) []solana.SignatureInfo {
		markDirty(t, pool, ata, 50)
		return nil
	}}
	if _, err := watchFor(pool, user, now, &rpc, unlimited(), 39, 40).Tick(watchActor(t)); err != nil {
		t.Fatal(err)
	}
	if got := loadAccount(t, pool, ata); got.CleanGen != 1 || got.DirtyGen != 2 {
		t.Fatalf("generations = clean %d dirty %d, want 1 and 2: the hint during the scan keeps the account dirty",
			got.CleanGen, got.DirtyGen)
	}
	if _, err := pool.Exec(t.Context(), `UPDATE deposit_watch_accounts SET clean_gen = 5, dirty_gen = 5`); err != nil {
		t.Fatal(err)
	}
	if err := sqlc.New(pool).CompleteDepositWatchPage(t.Context(), sqlc.CompleteDepositWatchPageParams{
		TokenAccount: string(ata), CleanGen: 2, ScannedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if got := loadAccount(t, pool, ata); got.CleanGen != 5 {
		t.Fatalf("clean_gen = %d, want 5: it only moves forward", got.CleanGen)
	}
}

func TestDepositWatchFixtureI17_FailedTransactionRecordsNoCandidate(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	rpc := watchRPC{signatures: []solana.SignatureInfo{{
		Signature: depositSignature, Slot: 42, Failed: true, BlockTime: depositBlockTime(),
	}}}
	p := newDepositWatch(t, pool, user, now, &rpc, 33, 34)
	ctx := watchActor(t)
	if report, err := p.Tick(ctx); err != nil || report.Changed != 0 {
		t.Fatalf("failed Tick = %+v, %v", report, err)
	}
	var candidates, seen int
	if err := pool.QueryRow(
		ctx,
		`SELECT (SELECT count(*) FROM deposit_candidates),
		(SELECT count(*) FROM events WHERE type = 'deposit.candidate_seen')`,
	).Scan(&candidates, &seen); err != nil || candidates != 0 || seen != 0 {
		t.Fatalf("candidates/seen events = %d/%d, %v; want 0/0", candidates, seen, err)
	}
	assertHighSignature(t, pool, user.Address, depositSignature)
}

func assertHighSignature(t *testing.T, pool *pgxpool.Pool, wallet chain.SolanaAddress, want string) {
	t.Helper()
	if got := loadAccount(t, pool, canonicalAccount(t, wallet)).High; got != want {
		t.Fatalf("high signature = %q; want %q", got, want)
	}
}

func TestDepositWatchFixtureI18_ReusedWalletRecordsNoCandidatesAtFirstSight(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	history := make([]solana.SignatureInfo, 0, 200)
	for i := 200; i > 0; i-- {
		history = append(history, solana.SignatureInfo{
			Signature: chain.Signature(fmt.Sprintf("hist%d", i)), Slot: uint64(i), BlockTime: depositBlockTime(),
		})
	}
	rpc := watchRPC{slot: 200, signaturesFor: func(before, until chain.Signature, limit int) []solana.SignatureInfo {
		return signaturesFromHistory(history, before, until, limit)
	}}
	p := watchFor(pool, user, now, &rpc, unlimited(), 78, 79)
	ctx := watchActor(t)
	for range 2 {
		if _, err := p.Tick(ctx); err != nil {
			t.Fatal(err)
		}
		assertCandidateSignatures(ctx, t, pool, user.Address)
	}
	var credited, deposits int
	var firstDeposit *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT (SELECT count(*) FROM events WHERE type = $1), (SELECT count(*) FROM deposits),
		(SELECT first_deposit_at FROM users WHERE id = $2)`, events.TypeDepositCredited, user.ID.UUID(),
	).Scan(&credited, &deposits, &firstDeposit); err != nil || credited != 0 || deposits != 0 || firstDeposit != nil {
		t.Fatalf("credited/deposits/first_deposit_at = %d/%d/%v, %v; want 0/0/nil",
			credited, deposits, firstDeposit, err)
	}
	assertHighSignature(t, pool, user.Address, "hist200")
}

func TestDepositWatchStampsTheScanTimeAfterAnEmptyScan(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	rpc := watchRPC{}
	p := newDepositWatch(t, pool, user, now, &rpc, 37, 38)
	if report, err := p.Tick(watchActor(t)); err != nil || report.Changed != 0 {
		t.Fatalf("Tick = %+v, %v; want no candidates", report, err)
	}
	got := loadAccount(t, pool, canonicalAccount(t, user.Address))
	if got.ScannedAt == nil || !got.ScannedAt.Equal(now) || got.CleanGen != got.DirtyGen {
		t.Fatalf("account = %+v, want scanned at %s and clean", got, now)
	}
}
