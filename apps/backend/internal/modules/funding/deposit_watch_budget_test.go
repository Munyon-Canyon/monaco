package funding_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func budgetedWatch(
	pool *pgxpool.Pool, now time.Time, wallets []identity.MemberWallet, rpc *watchRPC, calls int, uowID, watchID uint64,
) *app.DepositWatch {
	return budgetedWatchEvery(pool, now, wallets, rpc, calls, app.DepositPollInterval, uowID, watchID)
}

func budgetedWatchEvery(
	pool *pgxpool.Pool, now time.Time, wallets []identity.MemberWallet, rpc *watchRPC, calls int,
	period time.Duration, uowID, watchID uint64,
) *app.DepositWatch {
	rpc.pool = pool
	return app.NewDepositWatch(
		pool,
		db.New(pool, testkit.NewIDs(uowID), testkit.NewClock(now)),
		testkit.NewIDs(watchID),
		testkit.NewClock(now),
		fakes.NewIdentity(nil, wallets),
		rpc,
		testkit.USDCMint,
		period,
		unlimited(),
		calls,
		watchTuning(),
		noop.Int64Counter{}, &stubLedger{},
	)
}

func tickWithRemaining(t *testing.T, remaining time.Duration) (context.Context, time.Time) {
	t.Helper()
	deadline := clock.Real{}.Now().UTC().Add(30 * time.Minute).Truncate(time.Microsecond)
	ctx, cancel := context.WithDeadline(watchActor(t), deadline)
	t.Cleanup(cancel)
	return ctx, deadline.Add(-remaining)
}

func stepAttrs(report poller.Report) string {
	parts := make([]string, 0, len(report.Attrs))
	for _, a := range report.Attrs {
		parts = append(parts, a.Key+"="+a.Value.String())
	}
	return strings.Join(parts, " ")
}

func TestDepositWatchStopsAtTheCallBudgetAndScansTheOldestDirtyAccountFirst(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	newer := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	older := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	unseen := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	newerAccount := seedWatchAccount(t, pool, newer)
	olderAccount := seedWatchAccount(t, pool, older)
	markDirty(t, pool, newerAccount, 20)
	markDirty(t, pool, olderAccount, 10)
	wallets := []identity.MemberWallet{
		{UserID: newer.ID, Address: newer.Address},
		{UserID: older.ID, Address: older.Address},
		{UserID: unseen.ID, Address: unseen.Address},
	}
	rpc := watchRPC{}
	report, err := budgetedWatch(pool, now, wallets, &rpc, 2, 60, 61).Tick(watchActor(t))
	if err != nil || report.Scanned != 3 {
		t.Fatalf("Tick = %+v, %v; want two gated, one scanned and no error", report, err)
	}
	if got, want := stepAttrs(report), "gate=2 gate_calls=1 dirty=1 dirty_calls=1"; got != want {
		t.Fatalf("step attrs = %q, want %q: first sight must not run after the budget is spent", got, want)
	}
	if got := loadAccount(t, pool, olderAccount); got.CleanGen != got.DirtyGen {
		t.Fatalf("older account = %+v, want it scanned first", got)
	}
	if got := loadAccount(t, pool, newerAccount); got.CleanGen == got.DirtyGen {
		t.Fatalf("newer account = %+v, want it left dirty for the next tick", got)
	}
	var wallet int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM deposit_watch_wallets WHERE wallet_address = $1`,
		unseen.Address).Scan(&wallet); err != nil || wallet != 0 || rpc.signCalls != 1 {
		t.Fatalf("unseen wallet rows = %d, %v with %d signature calls; want 0 and 1", wallet, err, rpc.signCalls)
	}
}

func TestDepositWatchRunsFirstSightAfterTheDirtyCatchUpsAndReportsCallsPerStep(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	known := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	fresh := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	seedWatchAccount(t, pool, known)
	wallets := []identity.MemberWallet{
		{UserID: known.ID, Address: known.Address},
		{UserID: fresh.ID, Address: fresh.Address},
	}
	rpc := watchRPC{}
	report, err := budgetedWatch(pool, now, wallets, &rpc, 4, 62, 63).Tick(watchActor(t))
	if err != nil {
		t.Fatal(err)
	}
	want := "gate=1 gate_calls=1 dirty=1 dirty_calls=1 rotation=0 rotation_calls=0 " +
		"discovery=0 discovery_calls=0 first_sight=1 first_sight_calls=2 reconcile=0 reconcile_calls=0"
	if got := stepAttrs(report); got != want {
		t.Fatalf("step attrs = %q, want %q", got, want)
	}
}

func TestDepositWatchStopsFiveSecondsBeforeTheTickDeadline(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	ata := seedWatchAccount(t, pool, user)
	wallets := []identity.MemberWallet{{UserID: user.ID, Address: user.Address}}
	rpc := watchRPC{}
	late, lateNow := tickWithRemaining(t, 2*time.Second)
	report, err := budgetedWatch(pool, lateNow, wallets, &rpc, 480, 64, 65).Tick(late)
	if err != nil || rpc.signCalls != 0 || stepAttrs(report) != "gate=0 gate_calls=0" {
		t.Fatalf("late Tick = %+v, %v with %d calls; want no RPC call inside the last 5 s", report, err, rpc.signCalls)
	}
	if got := loadAccount(t, pool, ata); got.CleanGen == got.DirtyGen {
		t.Fatalf("account = %+v, want it still dirty", got)
	}
	early, earlyNow := tickWithRemaining(t, time.Minute)
	_, err = budgetedWatch(pool, earlyNow, wallets, &rpc, 480, 66, 67).Tick(early)
	if err != nil || rpc.signCalls != 1 {
		t.Fatalf("early Tick error = %v with %d calls; want one scan", err, rpc.signCalls)
	}
}

func TestDepositWatchStopsAFifthOfAShortIntervalBeforeTheTickDeadline(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	seedWatchAccount(t, pool, user)
	wallets := []identity.MemberWallet{{UserID: user.ID, Address: user.Address}}
	rpc := watchRPC{}
	late, lateNow := tickWithRemaining(t, 300*time.Millisecond)
	watch := budgetedWatchEvery(pool, lateNow, wallets, &rpc, 480, 2*time.Second, 68, 69)
	if _, err := watch.Tick(late); err != nil || rpc.signCalls != 0 {
		t.Fatalf("late Tick error = %v with %d calls; want none in the last 400ms of a 2s interval",
			err, rpc.signCalls)
	}
	early, earlyNow := tickWithRemaining(t, 500*time.Millisecond)
	watch = budgetedWatchEvery(pool, earlyNow, wallets, &rpc, 480, 2*time.Second, 70, 71)
	if _, err := watch.Tick(early); err != nil || rpc.signCalls != 1 {
		t.Fatalf("early Tick error = %v with %d calls; want one scan outside the 400ms margin", err, rpc.signCalls)
	}
}

func TestDepositWatchOverlappingTicksRecordOneCandidateAndKeepTheNewerDirtyGeneration(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	ata := seedWatchAccount(t, pool, user)
	wallets := []identity.MemberWallet{{UserID: user.ID, Address: user.Address}}
	page := []solana.SignatureInfo{{Signature: "overlap", Slot: 7, BlockTime: depositBlockTime()}}
	ctx := watchActor(t)
	second := watchRPC{signatures: page}
	secondReport := poller.Report{}
	nested := false
	first := watchRPC{}
	first.signaturesFor = func(chain.Signature, chain.Signature, int) []solana.SignatureInfo {
		if !nested {
			nested = true
			var err error
			if secondReport, err = budgetedWatch(pool, now, wallets, &second, 480, 70, 71).Tick(ctx); err != nil {
				t.Fatal(err)
			}
			hint := sqlc.MarkDepositWatchAccountDirtyParams{TokenAccount: string(ata), Slot: 50}
			if n, err := sqlc.New(pool).MarkDepositWatchAccountDirty(ctx, hint); err != nil || n != 1 {
				t.Fatalf("hint = %d, %v", n, err)
			}
		}
		return page
	}
	report, err := budgetedWatch(pool, now, wallets, &first, 480, 72, 73).Tick(ctx)
	if err != nil || report.Changed != 0 || secondReport.Changed != 1 {
		t.Fatalf("changed = %d then %d, %v; want the overlapping tick to insert nothing",
			secondReport.Changed, report.Changed, err)
	}
	assertCandidateSignatures(ctx, t, pool, user.Address, "overlap")
	var seen int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE type = 'deposit.candidate_seen'`).
		Scan(&seen); err != nil || seen != 1 {
		t.Fatalf("candidate_seen events = %d, %v; want 1", seen, err)
	}
	if got := loadAccount(t, pool, ata); got.CleanGen != 1 || got.DirtyGen != 2 {
		t.Fatalf("generations = clean %d dirty %d; want 1 and 2: the newer dirty generation survives",
			got.CleanGen, got.DirtyGen)
	}
}
