package funding_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

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
	return app.NewDepositWatch(
		pool,
		db.New(pool, testkit.NewIDs(uowID), testkit.NewClock(now)),
		testkit.NewIDs(watchID),
		testkit.NewClock(now),
		fakes.NewIdentity(nil, wallets),
		rpc,
		testkit.USDCMint,
		app.DepositPollInterval,
		unlimited(),
		calls,
	)
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
	report, err := budgetedWatch(pool, now, wallets, &rpc, 1, 60, 61).Tick(watchActor(t))
	if err != nil || report.Scanned != 1 {
		t.Fatalf("Tick = %+v, %v; want one account scanned and no error when the budget runs out", report, err)
	}
	if got, want := stepAttrs(report), "dirty=1 dirty_calls=1"; got != want {
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
	report, err := budgetedWatch(pool, now, wallets, &rpc, 3, 62, 63).Tick(watchActor(t))
	if err != nil {
		t.Fatal(err)
	}
	want := "dirty=1 dirty_calls=1 first_sight=1 first_sight_calls=2"
	if got := stepAttrs(report); got != want {
		t.Fatalf("step attrs = %q, want %q", got, want)
	}
}

func TestDepositWatchStopsFiveSecondsBeforeTheTickDeadline(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	ata := seedWatchAccount(t, pool, user)
	wallets := []identity.MemberWallet{{UserID: user.ID, Address: user.Address}}
	rpc := watchRPC{}
	late, cancel := context.WithTimeout(watchActor(t), 2*time.Second)
	defer cancel()
	report, err := budgetedWatch(pool, now, wallets, &rpc, 480, 64, 65).Tick(late)
	if err != nil || rpc.signCalls != 0 || stepAttrs(report) != "dirty=0 dirty_calls=0" {
		t.Fatalf("late Tick = %+v, %v with %d calls; want no RPC call inside the last 5 s", report, err, rpc.signCalls)
	}
	if got := loadAccount(t, pool, ata); got.CleanGen == got.DirtyGen {
		t.Fatalf("account = %+v, want it still dirty", got)
	}
	early, cancelEarly := context.WithTimeout(watchActor(t), time.Minute)
	defer cancelEarly()
	if _, err := budgetedWatch(pool, now, wallets, &rpc, 480, 66, 67).Tick(early); err != nil || rpc.signCalls != 1 {
		t.Fatalf("early Tick error = %v with %d calls; want one scan", err, rpc.signCalls)
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
