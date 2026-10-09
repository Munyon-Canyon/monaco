package funding_test

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const depositSignature = "5VERv8NMvzbJMEkV8xnrLkEaWRtSz9CosKDYjCJjBRnbJLgp8uirBgmQpjKhoR4tjF3ZpRzrFmBV6UjKdiSZkQUW"

func depositBlockTime() time.Time { return time.Unix(1_790_000_000, 0).UTC() }

type depositRPC struct {
	signatures      []solana.SignatureInfo
	err             error
	signErr         error
	signaturesFor   func(chain.Signature, chain.Signature, int) []solana.SignatureInfo
	signCalls       int
	signatureLimits []int
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
	ctx := observability.WithActor(t.Context(), "system:poller.funding.deposit_watch")
	if report, err := p.Tick(ctx); err != nil || report.Changed != 0 {
		t.Fatalf("first Tick = %+v, %v", report, err)
	}
	if report, err := p.Tick(ctx); err != nil || report.Changed != 1 {
		t.Fatalf("second Tick = %+v, %v", report, err)
	}
	if got, want := fmt.Sprint(rpc.signatureLimits), "[1 1000]"; got != want {
		t.Fatalf("signature limits = %s, want %s", got, want)
	}
	var candidates, deposits int
	var source string
	if err := pool.QueryRow(
		ctx,
		`SELECT count(*), coalesce(min(source), ''), (SELECT count(*) FROM deposits)
		FROM deposit_candidates WHERE wallet_address = $1`, user.Address,
	).Scan(&candidates, &source, &deposits); err != nil || candidates != 1 || source != "poller" || deposits != 0 {
		t.Fatalf("candidates/source/deposits = %d/%q/%d, %v; want 1/poller/0", candidates, source, deposits, err)
	}
	assertCandidateSignatures(ctx, t, pool, user.Address, "later")
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
	ctx := observability.WithActor(t.Context(), "system:poller.funding.deposit_watch")
	for _, allowance := range []int{3, 3, 2} {
		budget.refill(allowance)
		if _, err := p.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	assertBackfillProgress(ctx, t, pool, user.Address, rpc.signCalls, *beforeCalls, *untilCalls)
}

func TestDepositPollerCommitsCandidatesWithTheCheckpointOfAWholePage(t *testing.T) {
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
	pollerClock := testkit.NewClock(now)
	p := app.NewDepositPoller(
		pool, db.New(pool, testkit.NewIDs(76), pollerClock), testkit.NewIDs(77), pollerClock,
		fakes.NewIdentity(nil, []identity.MemberWallet{{UserID: user.ID, Address: user.Address}}),
		&rpc, testkit.USDCMint, app.DepositPollInterval, unlimited(),
	)
	ctx := observability.WithActor(t.Context(), "system:poller.funding.deposit_watch")
	pollerClock.Advance(time.Second)
	if report, err := p.Tick(ctx); err != nil || report.Changed != 3 {
		t.Fatalf("first Tick = %+v, %v; want three candidates", report, err)
	}
	assertBackfillFrontier(ctx, t, pool, user.Address, "sig1", "sig1000")
	assertCursorScannedAt(ctx, t, pool, user.Address, now.Add(time.Second))
	assertCandidateSignatures(ctx, t, pool, user.Address, "sig1000", "sig999", "sig998")
	if report, err := p.Tick(ctx); err != nil || report.Changed != 0 {
		t.Fatalf("second Tick = %+v, %v; want the walk finished", report, err)
	}
	assertFinishedWalk(ctx, t, pool, user.Address)
}

func assertFinishedWalk(ctx context.Context, t *testing.T, pool *pgxpool.Pool, address chain.SolanaAddress) {
	t.Helper()
	var deposits int
	var cursor, before string
	err := pool.QueryRow(
		ctx,
		`SELECT (SELECT count(*) FROM deposits WHERE wallet_address = $1), last_signature, backfill_before_signature
		FROM deposit_cursors WHERE wallet_address = $1`, address,
	).Scan(&deposits, &cursor, &before)
	if err != nil || deposits != 0 || cursor != "sig1000" || before != "" {
		t.Fatalf("deposits/cursor/before = %d/%q/%q, %v; want 0/sig1000/empty", deposits, cursor, before, err)
	}
}

func assertCandidateSignatures(
	ctx context.Context, t *testing.T, pool *pgxpool.Pool, address chain.SolanaAddress, want ...string,
) {
	t.Helper()
	rows, err := pool.Query(
		ctx,
		`SELECT tx_signature FROM deposit_candidates WHERE wallet_address = $1 AND status = 'pending'
		ORDER BY tx_signature`, address,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var signature string
		if err := rows.Scan(&signature); err != nil {
			t.Fatal(err)
		}
		got = append(got, signature)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("pending candidates = %v, want %v", got, want)
	}
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
	ctx := observability.WithActor(t.Context(), "system:poller.funding.deposit_watch")
	for range 3 {
		if _, err := p.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var deposits int
	if err := pool.QueryRow(
		ctx, `SELECT count(*) FROM deposits WHERE wallet_address = $1`, user.Address,
	).Scan(&deposits); err != nil {
		t.Fatal(err)
	}
	if deposits != 0 {
		t.Fatalf("deposits before the cursor = %d, want 0", deposits)
	}
	assertCandidateSignatures(ctx, t, pool, user.Address)
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

func TestDepositPollerReportsSignatureAndSlotFailures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		rpc  depositRPC
	}{
		{"signatures", depositRPC{signErr: errs.New(errs.CodeRPCUnavailable, "test.rpc")}},
		{"slot", depositRPC{signatures: []solana.SignatureInfo{{Signature: depositSignature, Slot: math.MaxInt64 + 1}}}},
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

func TestDepositPollerFixtureI17_FailedTransactionRecordsNoCandidate(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	rpc := depositRPC{signatures: []solana.SignatureInfo{{
		Signature: depositSignature, Slot: 42, Failed: true, BlockTime: depositBlockTime(),
	}}}
	p := newDepositPoller(t, pool, user, now, &rpc, 33, 34)
	ctx := observability.WithActor(t.Context(), "system:poller.funding.deposit_watch")
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
	assertCursorSignature(ctx, t, pool, user.Address, depositSignature)
}

func assertCursorSignature(
	ctx context.Context, t *testing.T, pool *pgxpool.Pool, address chain.SolanaAddress, want string,
) {
	t.Helper()
	var cursor string
	err := pool.QueryRow(
		ctx, `SELECT last_signature FROM deposit_cursors WHERE wallet_address = $1`, address,
	).Scan(&cursor)
	if err != nil || cursor != want {
		t.Fatalf("cursor = %q, %v; want %q", cursor, err, want)
	}
}

func TestDepositPollerReportsRecordFailureWithoutActor(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	rpc := depositRPC{
		signatures: []solana.SignatureInfo{{Signature: depositSignature, Slot: 42, BlockTime: depositBlockTime()}},
	}
	p := newDepositPoller(t, pool, user, now, &rpc, 35, 36)
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
	p := newDepositPoller(t, pool, user, now, &rpc, 37, 38)
	ctx := observability.WithActor(t.Context(), "system:poller.funding.deposit_watch")
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

func TestDepositPoller_recordsEverySignatureAndLeavesTokenChecksToTheResolver(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC()
	rpc := depositRPC{
		signatures: []solana.SignatureInfo{{Signature: depositSignature, Slot: 42, BlockTime: depositBlockTime()}},
	}
	p := newDepositPoller(t, pool, user, now, &rpc, 11, 12)
	ctx := observability.WithActor(t.Context(), "system:poller.funding.deposit_watch")
	report, err := p.Tick(ctx)
	if err != nil || report.Scanned != 1 || report.Changed != 1 {
		t.Fatalf("Tick = %+v, %v calls=%d", report, err, rpc.signCalls)
	}
	assertCursorSignature(ctx, t, pool, user.Address, depositSignature)
	assertCandidateSignatures(ctx, t, pool, user.Address, depositSignature)
}

func TestDepositPoller_recordsOneCandidateOnlyOnce(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC()
	rpc := depositRPC{
		signatures: []solana.SignatureInfo{{Signature: depositSignature, Slot: 42, BlockTime: depositBlockTime()}},
	}
	p := newDepositPoller(t, pool, user, now, &rpc, 13, 14)
	ctx := observability.WithActor(t.Context(), "system:poller.funding.deposit_watch")
	if report, err := p.Tick(ctx); err != nil || report.Changed != 1 {
		t.Fatalf("first Tick = %+v, %v calls=%d", report, err, rpc.signCalls)
	}
	if report, err := p.Tick(ctx); err != nil || report.Changed != 0 {
		t.Fatalf("second Tick = %+v, %v", report, err)
	}
	var candidates, seen, deposits int
	if err := pool.QueryRow(ctx,
		`SELECT (SELECT count(*) FROM deposit_candidates), (SELECT count(*) FROM events WHERE type = $1),
		(SELECT count(*) FROM deposits)`, events.TypeDepositCandidateSeen,
	).Scan(&candidates, &seen, &deposits); err != nil || candidates != 1 || seen != 1 || deposits != 0 {
		t.Fatalf("candidates/seen/deposits = %d/%d/%d, %v; want 1/1/0", candidates, seen, deposits, err)
	}
}

func TestDepositPollerFixtureI18_ReusedWalletRecordsNoCandidatesAtFirstSight(t *testing.T) {
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
	rpc := depositRPC{signaturesFor: func(before, until chain.Signature, limit int) []solana.SignatureInfo {
		return signaturesFromHistory(history, before, until, limit)
	}}
	p := app.NewDepositPoller(
		pool, db.New(pool, testkit.NewIDs(78), testkit.NewClock(now)), testkit.NewIDs(79), testkit.NewClock(now),
		fakes.NewIdentity(nil, []identity.MemberWallet{{UserID: user.ID, Address: user.Address}}),
		&rpc, testkit.USDCMint, app.DepositPollInterval, unlimited(),
	)
	ctx := observability.WithActor(t.Context(), "system:poller.funding.deposit_watch")
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
	assertCursorSignature(ctx, t, pool, user.Address, "hist200")
}

func TestDepositPoller_recordsACandidateWhenRPCOmitsBlockTime(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	rpc := depositRPC{
		signatures: []solana.SignatureInfo{{Signature: depositSignature, Slot: 42}},
	}
	p := newDepositPoller(t, pool, user, now, &rpc, 39, 40)
	ctx := observability.WithActor(t.Context(), "system:poller.funding.deposit_watch")
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
		ctx := observability.WithActor(t.Context(), "system:poller.funding.deposit_watch")
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
