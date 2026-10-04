package funding_test

import (
	"context"
	"fmt"
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
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const depositSignature = "5VERv8NMvzbJMEkV8xnrLkEaWRtSz9CosKDYjCJjBRnbJLgp8uirBgmQpjKhoR4tjF3ZpRzrFmBV6UjKdiSZkQUW"

func depositBlockTime() time.Time { return time.Unix(1_790_000_000, 0).UTC() }

type depositRPC struct {
	signatures      []solana.SignatureInfo
	accounts        []solana.TokenAccountState
	tokenSlot       uint64
	err             error
	signErr         error
	signaturesFor   func(chain.Signature, chain.Signature, int) []solana.SignatureInfo
	signCalls       int
	signatureLimits []int
	tokenErr        error
}

type cancelingWalletReader struct {
	wallet identity.MemberWallet
	cancel context.CancelFunc
}

func (r cancelingWalletReader) MemberWallet(context.Context, ids.UserID) (identity.MemberWallet, error) {
	return r.wallet, nil
}

func (r cancelingWalletReader) MemberWallets(context.Context, ids.UserID, int) ([]identity.MemberWallet, error) {
	r.cancel()
	return []identity.MemberWallet{r.wallet}, nil
}

func (r *depositRPC) TokenAccounts(
	context.Context, chain.SolanaAddress, chain.Mint,
) (uint64, []solana.TokenAccountState, error) {
	return r.tokenSlot, r.accounts, r.tokenErr
}

func signaturesFromHistory(
	history []solana.SignatureInfo, before, until chain.Signature, limit int,
) []solana.SignatureInfo {
	start, end := 0, len(history)
	for i, sig := range history {
		if sig.Signature == before {
			start = i + 1
		}
	}
	for i, sig := range history {
		if sig.Signature == until && i >= start {
			end = i
		}
	}
	page := history[start:end]
	if len(page) > limit {
		return page[:limit]
	}
	return page
}

func (r *depositRPC) SignaturesFor(
	_ context.Context,
	_ chain.SolanaAddress,
	opts solana.SignaturesOpts,
) ([]solana.SignatureInfo, error) {
	r.signCalls++
	r.signatureLimits = append(r.signatureLimits, opts.Limit)
	if r.signaturesFor != nil {
		return r.signaturesFor(opts.Before, opts.Until, opts.Limit), r.signErr
	}
	if r.signErr != nil {
		return nil, r.signErr
	}
	return r.signatures, r.err
}

func TestDepositPollerStartsNewWalletsAtTheChainTip(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	old := solana.SignatureInfo{Signature: "old", Slot: 1, BlockTime: depositBlockTime()}
	later := solana.SignatureInfo{Signature: "later", Slot: 2, BlockTime: depositBlockTime()}
	rpc := depositRPC{
		signaturesFor: func(_ chain.Signature, until chain.Signature, _ int) []solana.SignatureInfo {
			if until == "old" {
				return []solana.SignatureInfo{later}
			}
			return []solana.SignatureInfo{old}
		},
	}
	p := app.NewDepositPoller(
		pool, db.New(pool, testkit.NewIDs(70), testkit.NewClock(now)), testkit.NewIDs(71), testkit.NewClock(now),
		fakes.NewIdentity(nil, []identity.MemberWallet{{UserID: user.ID, Address: user.Address}}),
		&rpc, testkit.USDCMint, app.DepositPollInterval, unlimited(),
	)
	ctx := observability.WithActor(t.Context(), "system:poller.funding.deposits")
	if report, err := p.Tick(ctx); err != nil || report.Changed != 0 {
		t.Fatalf("first Tick = %+v, %v", report, err)
	}
	if report, err := p.Tick(ctx); err != nil || report.Changed != 1 {
		t.Fatalf("second Tick = %+v, %v", report, err)
	}
	if got, want := fmt.Sprint(rpc.signatureLimits), "[1 1000]"; got != want {
		t.Fatalf("signature limits = %s, want %s", got, want)
	}
	var count int
	if err := pool.QueryRow(
		ctx,
		`SELECT count(*) FROM deposit_candidates WHERE tx_signature = 'later'`,
	).Scan(&count); err != nil || count != 1 {
		t.Fatalf("later candidates = %d, %v", count, err)
	}
}

func TestDepositPollerResumesBackfillOnePageAtATime(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	if _, err := pool.Exec(
		t.Context(),
		`INSERT INTO deposit_cursors (wallet_address, last_signature, cursor_slot, scanned_at)
		VALUES ($1, 'baseline', 0, $2)`,
		user.Address,
		now,
	); err != nil {
		t.Fatal(err)
	}
	rpc, beforeCalls, untilCalls := pagedBackfillRPC()
	budget := &rpcBudget{}
	p := app.NewDepositPoller(
		pool, db.New(pool, testkit.NewIDs(72), testkit.NewClock(now)), testkit.NewIDs(73), testkit.NewClock(now),
		fakes.NewIdentity(nil, []identity.MemberWallet{{UserID: user.ID, Address: user.Address}}),
		&rpc, testkit.USDCMint, app.DepositPollInterval, budget,
	)
	ctx := observability.WithActor(t.Context(), "system:poller.funding.deposits")
	for _, allowance := range []int{3, 3, 2} {
		budget.refill(allowance)
		if _, err := p.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	assertBackfillProgress(ctx, t, pool, user.Address, rpc.signCalls, *beforeCalls, *untilCalls)
}

func TestDepositPollerCheckpointsCompletedSignaturesWhenTheTickEndsMidPage(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	if _, err := pool.Exec(
		t.Context(),
		`INSERT INTO deposit_cursors (wallet_address, last_signature, cursor_slot, scanned_at)
		VALUES ($1, 'baseline', 0, $2)`,
		user.Address,
		now,
	); err != nil {
		t.Fatal(err)
	}
	history := make([]solana.SignatureInfo, 0, 1000)
	for i := 1000; i > 0; i-- {
		history = append(history, solana.SignatureInfo{
			Signature: chain.Signature(fmt.Sprintf("sig%d", i)), Slot: uint64(i), Failed: true,
		})
	}
	for i := range 3 {
		history[i].Failed = false
	}
	rpc := depositRPC{
		signaturesFor: func(before, until chain.Signature, limit int) []solana.SignatureInfo {
			return signaturesFromHistory(history, before, until, limit)
		},
	}
	budget := &deadlineBudget{}
	pollerClock := testkit.NewClock(now)
	p := app.NewDepositPoller(
		pool, db.New(pool, testkit.NewIDs(76), pollerClock), testkit.NewIDs(77), pollerClock,
		fakes.NewIdentity(nil, []identity.MemberWallet{{UserID: user.ID, Address: user.Address}}),
		&rpc, testkit.USDCMint, app.DepositPollInterval, budget,
	)
	ctx, cancel := context.WithCancel(observability.WithActor(t.Context(), "system:poller.funding.deposits"))
	pollerClock.Advance(time.Second)
	budget.left, budget.cancel = 1, cancel
	if _, err := p.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	resumeCtx := observability.WithActor(t.Context(), "system:poller.funding.deposits")
	assertBackfillFrontier(resumeCtx, t, pool, user.Address, "sig1", "sig1000")
	assertCursorScannedAt(resumeCtx, t, pool, user.Address, now.Add(time.Second))
	budget.left = 2
	if _, err := p.Tick(resumeCtx); err != nil {
		t.Fatal(err)
	}
	assertMidPageProgress(resumeCtx, t, pool, user.Address)
}

func assertBackfillFrontier(
	ctx context.Context, t *testing.T, pool *pgxpool.Pool, address chain.SolanaAddress, wantBefore, wantHead string,
) {
	t.Helper()
	var before, head string
	err := pool.QueryRow(
		ctx,
		`SELECT backfill_before_signature, backfill_head_signature FROM deposit_cursors WHERE wallet_address = $1`,
		address,
	).Scan(&before, &head)
	if err != nil || before != wantBefore || head != wantHead {
		t.Fatalf("frontier = %q/%q, %v; want %s/%s", before, head, err, wantBefore, wantHead)
	}
}

func assertCursorScannedAt(
	ctx context.Context, t *testing.T, pool *pgxpool.Pool, address chain.SolanaAddress, want time.Time,
) {
	t.Helper()
	var got time.Time
	err := pool.QueryRow(
		ctx, `SELECT scanned_at FROM deposit_cursors WHERE wallet_address = $1`, address,
	).Scan(&got)
	if err != nil || !got.Equal(want) {
		t.Fatalf("scanned_at = %s, %v; want %s", got, err, want)
	}
}

func assertMidPageProgress(ctx context.Context, t *testing.T, pool *pgxpool.Pool, address chain.SolanaAddress) {
	t.Helper()
	var deposits int
	err := pool.QueryRow(
		ctx, `SELECT count(*) FROM deposit_candidates WHERE wallet_address = $1`, address,
	).Scan(&deposits)
	if err != nil || deposits != 3 {
		t.Fatalf("candidates = %d, %v; want 3", deposits, err)
	}
	var cursor string
	err = pool.QueryRow(
		ctx, `SELECT last_signature FROM deposit_cursors WHERE wallet_address = $1`, address,
	).Scan(&cursor)
	if err != nil || cursor != "sig1000" {
		t.Fatalf("cursor = %q, %v; want sig1000", cursor, err)
	}
}

func TestDepositPollerDoesNotCreditBeforeTheCursorAfterACompletedBackfillCrash(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	if _, err := pool.Exec(
		t.Context(),
		`INSERT INTO deposit_cursors (
			wallet_address, last_signature, cursor_slot, scanned_at,
			backfill_before_signature, backfill_head_signature, backfill_head_slot
		) VALUES ($1, 'sig10', 10, $2, 'sig9', 'sig10', 10)`,
		user.Address,
		now,
	); err != nil {
		t.Fatal(err)
	}
	history := []solana.SignatureInfo{
		{Signature: "sig10", Slot: 10, BlockTime: depositBlockTime()},
		{Signature: "sig9", Slot: 9, BlockTime: depositBlockTime()},
		{Signature: "old", Slot: 8, BlockTime: depositBlockTime()},
		{Signature: "before-registration-1", Slot: 5, BlockTime: depositBlockTime()},
		{Signature: "before-registration-2", Slot: 4, BlockTime: depositBlockTime()},
	}
	rpc := depositRPC{
		signaturesFor: func(before, until chain.Signature, limit int) []solana.SignatureInfo {
			return signaturesFromHistory(history, before, until, limit)
		},
	}
	p := app.NewDepositPoller(
		pool, db.New(pool, testkit.NewIDs(74), testkit.NewClock(now)), testkit.NewIDs(75), testkit.NewClock(now),
		fakes.NewIdentity(nil, []identity.MemberWallet{{UserID: user.ID, Address: user.Address}}),
		&rpc, testkit.USDCMint, app.DepositPollInterval, unlimited(),
	)
	ctx := observability.WithActor(t.Context(), "system:poller.funding.deposits")
	for range 3 {
		if _, err := p.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var deposits int
	if err := pool.QueryRow(
		ctx, `SELECT count(*) FROM deposit_candidates WHERE wallet_address = $1`, user.Address,
	).Scan(&deposits); err != nil {
		t.Fatal(err)
	}
	if deposits != 0 {
		t.Fatalf("deposits before the cursor = %d, want 0", deposits)
	}
}

func pagedBackfillRPC() (depositRPC, *[]chain.Signature, *[]chain.Signature) {
	pages := map[chain.Signature][]solana.SignatureInfo{}
	creditable := map[chain.Signature]bool{
		"sig2500": true,
		"sig1501": true,
		"sig1500": true,
		"sig501":  true,
		"sig1":    true,
	}
	for page, start := range []int{2500, 1500, 500} {
		items := make([]solana.SignatureInfo, 0, 1000)
		for i := start; i > start-1000 && i > 0; i-- {
			signature := chain.Signature(fmt.Sprintf("sig%d", i))
			items = append(items, solana.SignatureInfo{
				Signature: signature,
				Slot:      uint64(i),
				Failed:    !creditable[signature],
			})
		}
		if page == 0 {
			pages[""] = items
		} else {
			pages[chain.Signature(fmt.Sprintf("sig%d", start+1))] = items
		}
	}
	var beforeCalls []chain.Signature
	var untilCalls []chain.Signature
	rpc := depositRPC{
		signaturesFor: func(before, until chain.Signature, _ int) []solana.SignatureInfo {
			beforeCalls = append(beforeCalls, before)
			untilCalls = append(untilCalls, until)
			page := pages[before]
			for i, sig := range page {
				if sig.Signature == until {
					return page[:i]
				}
			}
			return page
		},
	}
	return rpc, &beforeCalls, &untilCalls
}

func assertBackfillProgress(
	ctx context.Context,
	t *testing.T,
	pool *pgxpool.Pool,
	address chain.SolanaAddress,
	calls int,
	beforeCalls []chain.Signature,
	untilCalls []chain.Signature,
) {
	t.Helper()
	if calls != 3 {
		t.Fatalf("SignaturesFor calls = %d, want 3", calls)
	}
	if got, want := fmt.Sprint(beforeCalls), "[ sig1501 sig501]"; got != want {
		t.Fatalf("before calls = %s, want %s", got, want)
	}
	if got, want := fmt.Sprint(untilCalls), "[baseline baseline baseline]"; got != want {
		t.Fatalf("until calls = %s, want %s", got, want)
	}
	rows, err := pool.Query(
		ctx,
		`SELECT tx_signature, count(*) FROM deposit_candidates WHERE wallet_address = $1 GROUP BY tx_signature`,
		address,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]int{}
	for rows.Next() {
		var (
			signature string
			count     int
		)
		if err := rows.Scan(&signature, &count); err != nil {
			t.Fatal(err)
		}
		got[signature] = count
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"sig2500": 1, "sig1501": 1, "sig1500": 1, "sig501": 1, "sig1": 1}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
	var cursor string
	if err := pool.QueryRow(
		ctx,
		`SELECT last_signature FROM deposit_cursors WHERE wallet_address = $1`,
		address,
	).Scan(&cursor); err != nil || cursor != "sig2500" {
		t.Fatalf("cursor = %q, %v, want sig2500", cursor, err)
	}
}

func TestDepositPollerReportsSignatureFailures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		rpc  depositRPC
	}{
		{"signatures", depositRPC{signErr: errs.New(errs.CodeRPCUnavailable, "test.rpc")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := testkit.DB(t)
			user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
			now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
			p := newDepositPoller(t, pool, user, now, &tc.rpc, 31, 32)
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

type deadlineBudget struct {
	left   int
	cancel context.CancelFunc
}

func (b *deadlineBudget) Wait(context.Context) error {
	if b.left == 0 {
		b.cancel()
		return context.Canceled
	}
	b.left--
	return nil
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
	t *testing.T,
	pool *pgxpool.Pool,
	user testkit.SeededUser,
	now time.Time,
	rpc *depositRPC,
	uowID, pollerID uint64,
) *app.DepositPoller {
	t.Helper()
	if _, err := pool.Exec(
		t.Context(),
		`INSERT INTO deposit_cursors (wallet_address, last_signature, cursor_slot, scanned_at)
		VALUES ($1, 'baseline', 1, $2)`,
		user.Address,
		now,
	); err != nil {
		t.Fatal(err)
	}
	return app.NewDepositPoller(
		pool, db.New(pool, testkit.NewIDs(uowID), testkit.NewClock(now)),
		testkit.NewIDs(pollerID), testkit.NewClock(now),
		fakes.NewIdentity(nil, []identity.MemberWallet{{UserID: user.ID, Address: user.Address}}),
		rpc, testkit.USDCMint, app.DepositPollInterval, unlimited(),
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
	p := newDepositPoller(t, pool, user, now, &rpc, 33, 34)
	if got := p.Name(); got != "funding.deposits" {
		t.Fatalf("Name = %q", got)
	}
	if report, err := p.Tick(t.Context()); err != nil || report.Changed != 0 {
		t.Fatalf("failed Tick = %+v, %v", report, err)
	}
	rpc.signatures = []solana.SignatureInfo{{
		Signature: "overflow", Slot: math.MaxUint64, BlockTime: depositBlockTime(),
	}}
	if _, err := p.Tick(t.Context()); err == nil {
		t.Fatal("overflow Tick error = nil")
	}
}

func TestDepositPollerSurfacesAdvanceFailures(t *testing.T) {
	t.Parallel()
	runDepositPollerCheckpointFailure(
		t,
		1,
		"NEW.backfill_before_signature IS NOT DISTINCT FROM OLD.backfill_before_signature",
	)
}

func TestDepositPollerSurfacesCheckpointFailures(t *testing.T) {
	t.Parallel()
	runDepositPollerCheckpointFailure(
		t,
		1000,
		"NEW.backfill_before_signature IS DISTINCT FROM OLD.backfill_before_signature",
	)
}

func TestDepositPollerSurfacesTouchFailures(t *testing.T) {
	t.Parallel()
	runDepositPollerCheckpointFailure(
		t,
		1000,
		"NEW.backfill_before_signature IS NOT DISTINCT FROM OLD.backfill_before_signature",
	)
}

func runDepositPollerCheckpointFailure(t *testing.T, count int, condition string) {
	t.Helper()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := time.Unix(1_790_000_000, 0).UTC()
	rpc := &depositRPC{signatures: make([]solana.SignatureInfo, count)}
	for i := range rpc.signatures {
		rpc.signatures[i] = solana.SignatureInfo{
			Signature: chain.Signature(fmt.Sprintf("checkpoint-%d", i)),
			Slot:      1,
		}
	}
	poller := newDepositPoller(t, pool, user, now, rpc, 811, 812)
	if _, err := pool.Exec(
		t.Context(),
		`UPDATE deposit_cursors SET backfill_before_signature = 'pending', backfill_head_signature = 'head', backfill_head_slot = 1 WHERE wallet_address = $1`,
		user.Address,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(
		t.Context(),
		`CREATE FUNCTION fail_deposit_checkpoint() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fail'; END $$;
		 CREATE TRIGGER fail_deposit_checkpoint BEFORE UPDATE ON deposit_cursors FOR EACH ROW WHEN (`+condition+`) EXECUTE FUNCTION fail_deposit_checkpoint()`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := poller.Tick(t.Context()); err == nil {
		t.Fatal("Tick error = nil")
	}
}

func TestDepositPollerRecordsCandidatesWithoutActor(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	rpc := depositRPC{
		signatures: []solana.SignatureInfo{{Signature: depositSignature, Slot: 42, BlockTime: depositBlockTime()}},
	}
	p := newDepositPoller(t, pool, user, now, &rpc, 35, 36)
	if report, err := p.Tick(t.Context()); err != nil || report.Changed != 1 {
		t.Fatalf("Tick = %+v, %v", report, err)
	}
}

func TestDepositPollerSurfacesCandidateRecordingFailures(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := time.Unix(1_790_000_000, 0).UTC()
	poller := newDepositPoller(
		t,
		pool,
		user,
		now,
		&depositRPC{signatures: []solana.SignatureInfo{{Signature: "record-failure", Slot: 1}}},
		813,
		814,
	)
	if _, err := pool.Exec(
		t.Context(),
		`CREATE FUNCTION fail_poller_candidate() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fail'; END $$;
		 CREATE TRIGGER fail_poller_candidate BEFORE INSERT ON deposit_candidates FOR EACH ROW EXECUTE FUNCTION fail_poller_candidate()`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := poller.Tick(t.Context()); err == nil {
		t.Fatal("Tick error = nil")
	}
}

func TestDepositPollerTouchesCursorAfterAnEmptyScan(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	rpc := depositRPC{}
	p := newDepositPoller(t, pool, user, now, &rpc, 37, 38)
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

func TestDepositPoller_recordsCandidatesAndAdvancesPastOtherTokens(t *testing.T) {
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
	}
	p := newDepositPoller(t, pool, user, now, &rpc, 11, 12)
	report, err := p.Tick(t.Context())
	if err != nil || report.Scanned != 1 || report.Changed != 1 {
		t.Fatalf("Tick = %+v, %v calls=%d", report, err, rpc.signCalls)
	}
	var cursor string
	const readCursor = `SELECT last_signature FROM deposit_cursors WHERE wallet_address = $1`
	err = pool.QueryRow(t.Context(), readCursor, user.Address).Scan(&cursor)
	if err != nil || cursor != depositSignature {
		t.Fatalf("cursor = %q, %v", cursor, err)
	}
}

func TestDepositPoller_recordsOneCandidateOnlyOnce(t *testing.T) {
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
	}
	p := newDepositPoller(t, pool, user, now, &rpc, 13, 14)
	ctx := observability.WithActor(t.Context(), "system:poller.funding.deposits")
	if report, err := p.Tick(ctx); err != nil || report.Changed != 1 {
		t.Fatalf("first Tick = %+v, %v calls=%d", report, err, rpc.signCalls)
	}
	if report, err := p.Tick(ctx); err != nil || report.Changed != 0 {
		t.Fatalf("second Tick = %+v, %v", report, err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM deposit_candidates`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("candidates = %d, %v", count, err)
	}
}

func TestDepositPoller_recordsCandidateWhenRPCOmitsBlockTime(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	rpc := depositRPC{
		signatures: []solana.SignatureInfo{{Signature: depositSignature, Slot: 42}},
	}
	p := newDepositPoller(t, pool, user, now, &rpc, 39, 40)
	ctx := observability.WithActor(t.Context(), "system:poller.funding.deposits")
	if report, err := p.Tick(ctx); err != nil || report.Changed != 1 {
		t.Fatalf("Tick = %+v, %v", report, err)
	}
	var blockTime *time.Time
	err := pool.QueryRow(ctx, `SELECT block_time FROM deposit_candidates WHERE tx_signature = $1`, depositSignature).
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
	context.Context,
	chain.SolanaAddress,
	solana.SignaturesOpts,
) ([]solana.SignatureInfo, error) {
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
		fakes.NewIdentity(nil, wallets), emptyRPC{}, testkit.USDCMint, app.DepositPollInterval, budget,
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

func newDepositWatch(
	t *testing.T,
	pool *pgxpool.Pool,
	wallets []identity.MemberWallet,
	rpc *depositRPC,
	now time.Time,
	period time.Duration,
	limit app.RPCLimiter,
) *app.DepositWatch {
	t.Helper()
	return app.NewDepositWatch(
		pool,
		db.New(pool, testkit.NewIDs(801), testkit.NewClock(now)),
		testkit.NewIDs(802),
		testkit.NewClock(now),
		fakes.NewIdentity(nil, wallets),
		rpc,
		testkit.USDCMint,
		period,
		10,
		limit,
	)
}

func seedDirtyWatch(t *testing.T, pool *pgxpool.Pool, user testkit.SeededUser, account string) {
	t.Helper()
	if _, err := pool.Exec(
		t.Context(),
		`INSERT INTO deposit_watch_wallets (wallet_address, user_id, first_seen_slot, first_seen_at)
		 VALUES ($1, $2, 0, now())`, user.Address, user.ID.UUID(),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(
		t.Context(),
		`INSERT INTO deposit_watch_accounts (token_account, wallet_address, canonical, state, dirty_gen, recovery_due_at)
		 VALUES ($1, $2, true, 'open', 1, now())`,
		account,
		user.Address,
	); err != nil {
		t.Fatal(err)
	}
}

func TestDepositWatchCoversFirstSightEdgeCases(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	now := time.Unix(1_790_000_000, 0).UTC()
	watch := func(user testkit.SeededUser, rpc *depositRPC, limit app.RPCLimiter) *app.DepositWatch {
		return newDepositWatch(
			t, pool, []identity.MemberWallet{{UserID: user.ID, Address: user.Address}}, rpc, now, time.Minute, limit,
		)
	}
	seeded := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	if _, err := watch(seeded, &depositRPC{tokenSlot: 1}, unlimited()).Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	limitedWait := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	if _, err := watch(limitedWait, &depositRPC{tokenSlot: 1}, &rpcBudget{}).Tick(t.Context()); err == nil {
		t.Fatal("seed wait error = nil")
	}
	invalid := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	if _, err := newDepositWatch(
		t, pool, []identity.MemberWallet{{UserID: invalid.ID, Address: "not-a-solana-address"}},
		&depositRPC{tokenSlot: 1}, now, time.Minute, unlimited(),
	).Tick(t.Context()); err == nil {
		t.Fatal("invalid wallet address error = nil")
	}
	account := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	if _, err := watch(
		account,
		&depositRPC{
			tokenSlot:  1,
			accounts:   []solana.TokenAccountState{{Address: "noncanonical", Exists: true}},
			signatures: []solana.SignatureInfo{{Signature: "tip", Slot: 1}},
		},
		unlimited(),
	).Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	highSlot := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	if _, err := watch(
		highSlot,
		&depositRPC{tokenSlot: 1, signatures: []solana.SignatureInfo{{Signature: "tip", Slot: math.MaxInt64 + 1}}},
		unlimited(),
	).Tick(t.Context()); err == nil {
		t.Fatal("high signature slot error = nil")
	}
	limited := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	if _, err := watch(limited, &depositRPC{tokenSlot: 1}, &rpcBudget{left: 1}).Tick(t.Context()); err == nil {
		t.Fatal("limited signature request error = nil")
	}
	overflow := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	if _, err := watch(overflow, &depositRPC{tokenSlot: math.MaxInt64 + 1}, unlimited()).Tick(t.Context()); err == nil {
		t.Fatal("seed slot overflow error = nil")
	}
	failing := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	if _, err := pool.Exec(
		t.Context(),
		`CREATE FUNCTION fail_watch_account() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fail'; END $$;
		 CREATE TRIGGER fail_watch_account BEFORE INSERT ON deposit_watch_accounts FOR EACH ROW EXECUTE FUNCTION fail_watch_account()`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := watch(failing, &depositRPC{tokenSlot: 1}, unlimited()).Tick(t.Context()); err == nil {
		t.Fatal("watch account insert error = nil")
	}
	failingWallet := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	if _, err := pool.Exec(
		t.Context(),
		`CREATE FUNCTION fail_watch_wallet() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fail'; END $$;
		 CREATE TRIGGER fail_watch_wallet BEFORE INSERT ON deposit_watch_wallets FOR EACH ROW EXECUTE FUNCTION fail_watch_wallet()`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := watch(failingWallet, &depositRPC{tokenSlot: 1}, unlimited()).Tick(t.Context()); err == nil {
		t.Fatal("watch wallet insert error = nil")
	}
}

func TestDepositWatchSurfacesMemberWalletsErrors(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	identityReader := fakes.NewIdentity(
		nil,
		[]identity.MemberWallet{{UserID: user.ID, Address: user.Address}},
	)
	identityReader.Fail("MemberWallets", errs.New(errs.CodeInternal, "test.wallets"))
	now := time.Unix(1_790_000_000, 0).UTC()
	watch := app.NewDepositWatch(
		pool,
		db.New(pool, testkit.NewIDs(805), testkit.NewClock(now)),
		testkit.NewIDs(806),
		testkit.NewClock(now),
		identityReader,
		&depositRPC{},
		testkit.USDCMint,
		time.Minute,
		10,
		unlimited(),
	)
	if _, err := watch.Tick(t.Context()); err == nil {
		t.Fatal("member wallets error = nil")
	}
}

func TestDepositWatchSurfacesCanceledDatabaseReads(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	now := time.Unix(1_790_000_000, 0).UTC()
	watch := app.NewDepositWatch(
		pool,
		db.New(pool, testkit.NewIDs(807), testkit.NewClock(now)),
		testkit.NewIDs(808),
		testkit.NewClock(now),
		cancelingWalletReader{wallet: identity.MemberWallet{UserID: user.ID, Address: user.Address}, cancel: cancel},
		&depositRPC{},
		testkit.USDCMint,
		time.Minute,
		10,
		unlimited(),
	)
	if _, err := watch.Tick(ctx); err == nil {
		t.Fatal("Tick with canceled read = nil")
	}
}

func TestDepositWatchSurfacesCanceledDirtyReads(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	watch := newDepositWatch(t, pool, nil, &depositRPC{}, clock.Real{}.Now(), time.Minute, unlimited())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := watch.Tick(ctx); err == nil {
		t.Fatal("Tick with canceled dirty read = nil")
	}
}

func TestDepositWatchStopsAtItsContextDeadline(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	now := clock.Real{}.Now()
	watch := newDepositWatch(t, pool, nil, &depositRPC{}, now, time.Minute, unlimited())
	ctx, cancel := context.WithDeadline(t.Context(), now.Add(time.Second))
	defer cancel()
	if report, err := watch.Tick(ctx); err != nil || report.Scanned != 0 {
		t.Fatalf("Tick = %+v, %v; want no work before the deadline", report, err)
	}
}

func TestDepositWatchStopsDirtyPagesWhenTimeBudgetExpires(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	first := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	second := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	seedDirtyWatch(t, pool, first, "time-first")
	seedDirtyWatch(t, pool, second, "time-second")
	now := time.Unix(1_790_000_000, 0).UTC()
	clk := testkit.NewClock(now)
	rpc := &depositRPC{signaturesFor: func(chain.Signature, chain.Signature, int) []solana.SignatureInfo {
		clk.Advance(time.Hour)
		return nil
	}}
	watch := app.NewDepositWatch(
		pool, db.New(pool, testkit.NewIDs(809), clk), testkit.NewIDs(810), clk,
		fakes.NewIdentity(nil, nil), rpc, testkit.USDCMint, time.Minute, 2, unlimited(),
	)
	if report, err := watch.Tick(t.Context()); err != nil || report.Scanned != 1 {
		t.Fatalf("Tick = %+v, %v; want one dirty account", report, err)
	}
}

func TestDepositWatchCoversPersistedEdgeCases(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := time.Unix(1_790_000_000, 0).UTC()
	newWatch := func(rpc *depositRPC, period time.Duration) *app.DepositWatch {
		return newDepositWatch(
			t, pool, []identity.MemberWallet{{UserID: user.ID, Address: user.Address}}, rpc, now, period, unlimited(),
		)
	}
	if _, err := newWatch(&depositRPC{}, time.Second).Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := newWatch(
		&depositRPC{tokenErr: errs.New(errs.CodeRPCUnavailable, "test.token")},
		time.Minute,
	).Tick(t.Context()); err == nil {
		t.Fatal("token error = nil")
	}
	if _, err := newWatch(
		&depositRPC{tokenSlot: 1, signErr: errs.New(errs.CodeRPCUnavailable, "test.sign")},
		time.Minute,
	).Tick(t.Context()); err == nil {
		t.Fatal("signature error = nil")
	}
	ata, err := chain.AssociatedTokenAccount(user.Address, testkit.USDCMint, chain.SPLProgram)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(
		t.Context(),
		`INSERT INTO deposit_watch_wallets (wallet_address, user_id, first_seen_slot, first_seen_at)
		 VALUES ($1, $2, 0, now()) ON CONFLICT (wallet_address) DO NOTHING`,
		user.Address,
		user.ID.UUID(),
	); err != nil {
		t.Fatal(err)
	}
	seedDirty := func(account string) {
		t.Helper()
		if _, err := pool.Exec(
			t.Context(),
			`INSERT INTO deposit_watch_accounts (token_account, wallet_address, canonical, state, dirty_gen, recovery_due_at) VALUES ($1, $2, true, 'open', 1, now())`,
			account,
			user.Address,
		); err != nil {
			t.Fatal(err)
		}
	}
	seedDirty(string(ata))
	if _, err := newWatch(&depositRPC{signatures: nil}, time.Minute).Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	seedDirty("overflow")
	if _, err := newWatch(
		&depositRPC{signatures: []solana.SignatureInfo{{Signature: "overflow", Slot: math.MaxInt64 + 1}}},
		time.Minute,
	).Tick(t.Context()); err == nil {
		t.Fatal("overflow error = nil")
	}
	seedDirty("candidate-error")
	if _, err := pool.Exec(
		t.Context(),
		`CREATE FUNCTION fail_candidate() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fail'; END $$; CREATE TRIGGER fail_candidate BEFORE INSERT ON deposit_candidates FOR EACH ROW EXECUTE FUNCTION fail_candidate()`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := newWatch(
		&depositRPC{signatures: []solana.SignatureInfo{{Signature: "candidate", Slot: 1}}},
		time.Minute,
	).Tick(t.Context()); err == nil {
		t.Fatal("candidate error = nil")
	}
}

func TestDepositWatchStopsDirtyPagesAfterItsCallBudget(t *testing.T) {
	t.Parallel()
	now, pool := time.Unix(1_790_000_000, 0).UTC(), testkit.DB(t)
	first := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	second := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	seedDirtyWatch(t, pool, first, "first")
	seedDirtyWatch(t, pool, second, "second")
	watch := app.NewDepositWatch(
		pool, db.New(pool, testkit.NewIDs(803), testkit.NewClock(now)), testkit.NewIDs(804), testkit.NewClock(now),
		fakes.NewIdentity(nil, nil), &depositRPC{}, testkit.USDCMint, time.Minute, 1, unlimited(),
	)
	if report, err := watch.Tick(t.Context()); err != nil || report.Scanned != 2 {
		t.Fatalf("Tick = %+v, %v; want two dirty accounts", report, err)
	}
}

func TestDepositWatchRejectsNegativeDirtySlots(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	seedDirtyWatch(t, pool, user, "negative-slot")
	if _, err := pool.Exec(
		t.Context(),
		`UPDATE deposit_watch_accounts SET dirty_slot = -1 WHERE token_account = 'negative-slot'`,
	); err != nil {
		t.Fatal(err)
	}
	watch := newDepositWatch(t, pool, nil, &depositRPC{}, time.Unix(1_790_000_000, 0).UTC(), time.Minute, unlimited())
	if _, err := watch.Tick(t.Context()); err == nil {
		t.Fatal("negative dirty slot error = nil")
	}
}

func TestDepositWatchSurfacesCheckpointFailures(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	seedDirtyWatch(t, pool, user, "checkpoint-failure")
	if _, err := pool.Exec(
		t.Context(),
		`CREATE FUNCTION fail_watch_checkpoint() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fail'; END $$;
		 CREATE TRIGGER fail_watch_checkpoint BEFORE UPDATE ON deposit_watch_accounts FOR EACH ROW EXECUTE FUNCTION fail_watch_checkpoint()`,
	); err != nil {
		t.Fatal(err)
	}
	watch := newDepositWatch(
		t,
		pool,
		nil,
		&depositRPC{signatures: []solana.SignatureInfo{{Signature: "checkpoint", Slot: 1}}},
		time.Unix(1_790_000_000, 0).UTC(),
		time.Minute,
		unlimited(),
	)
	if _, err := watch.Tick(t.Context()); err == nil {
		t.Fatal("checkpoint error = nil")
	}
}

func TestDepositWatchKeepsFullPagesDirtyForContinuation(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	seedDirtyWatch(t, pool, user, "full-page")
	page := make([]solana.SignatureInfo, 1000)
	for i := range page {
		page[i] = solana.SignatureInfo{Signature: chain.Signature(fmt.Sprintf("full-%d", i)), Slot: 1}
	}
	watch := newDepositWatch(
		t, pool, nil, &depositRPC{signatures: page}, time.Unix(1_790_000_000, 0).UTC(), time.Minute, unlimited(),
	)
	if report, err := watch.Tick(t.Context()); err != nil || report.Scanned != 1 {
		t.Fatalf("Tick = %+v, %v; want continued dirty page", report, err)
	}
}
