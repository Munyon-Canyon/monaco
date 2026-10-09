package funding_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type firstSightEnv struct {
	pool    *pgxpool.Pool
	user    testkit.SeededUser
	ledger  *stubLedger
	watch   *app.DepositWatch
	rpc     *watchRPC
	created time.Time
	now     time.Time
	runs    uint64
}

func newFirstSightEnv(t *testing.T, created time.Time, history []solana.SignatureInfo) *firstSightEnv {
	t.Helper()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	rpc := &watchRPC{
		pool: pool,
		slot: 10,
		accounts: []solana.TokenAccountState{{
			Address: canonicalAccount(t, user.Address), Exists: true, Amount: money.NewBaseUnits(7, 6),
		}},
		signaturesFor: func(before, until chain.Signature, limit int) []solana.SignatureInfo {
			return signaturesFromHistory(history, before, until, limit)
		},
	}
	e := &firstSightEnv{pool: pool, user: user, ledger: &stubLedger{}, rpc: rpc, created: created, now: now}
	e.restart()
	return e
}

func (e *firstSightEnv) restart() {
	e.runs++
	e.watch = app.NewDepositWatch(
		e.pool,
		db.New(e.pool, testkit.NewIDs(150+e.runs*2), testkit.NewClock(e.now)),
		testkit.NewIDs(151+e.runs*2),
		testkit.NewClock(e.now),
		fakes.NewIdentity(
			nil,
			[]identity.MemberWallet{{UserID: e.user.ID, Address: e.user.Address, CreatedAt: e.created}},
		),
		e.rpc,
		testkit.USDCMint,
		app.DepositPollInterval,
		unlimited(),
		480,
		watchTuning(),
		noop.Int64Counter{},
		e.ledger,
	)
}

func (e *firstSightEnv) tick(t *testing.T) {
	t.Helper()
	if _, err := e.watch.Tick(watchActor(t)); err != nil {
		t.Fatal(err)
	}
}

func (e *firstSightEnv) opening(t *testing.T) *string {
	t.Helper()
	var opening *string
	if err := e.pool.QueryRow(t.Context(), `SELECT opening_micros::text FROM deposit_watch_wallets`).
		Scan(&opening); err != nil {
		t.Fatal(err)
	}
	return opening
}

func TestDepositWatchFirstSightRecordsSignaturesFromTheWalletCreationExactlyOnce(t *testing.T) {
	t.Parallel()
	created := clock.Real{}.Now().UTC().Add(-time.Hour)
	history := []solana.SignatureInfo{
		{Signature: "above", Slot: 11, BlockTime: created.Add(50 * time.Minute)},
		{Signature: "deposit", Slot: 9, BlockTime: created.Add(30 * time.Minute)},
		{Signature: "failed", Slot: 8, BlockTime: created.Add(20 * time.Minute), Failed: true},
		{Signature: "skew", Slot: 7, BlockTime: created.Add(-4 * time.Minute)},
		{Signature: "history", Slot: 6, BlockTime: created.Add(-6 * time.Minute)},
		{Signature: "older", Slot: 5, BlockTime: created.Add(-time.Hour)},
	}
	e := newFirstSightEnv(t, created, history)
	e.tick(t)
	assertCandidateSignatures(t.Context(), t, e.pool, e.user.Address, "deposit", "skew")
	var high string
	if err := e.pool.QueryRow(t.Context(), `SELECT high_signature FROM deposit_watch_accounts`).
		Scan(&high); err != nil ||
		high != "deposit" {
		t.Fatalf("high_signature = %q, %v; want deposit, the newest signature at or below the context slot", high, err)
	}
	e.tick(t)
	assertCandidateSignatures(t.Context(), t, e.pool, e.user.Address, "deposit", "skew")
	if n := candidateCount(t, e.pool, ""); n != 2 {
		t.Fatalf("candidates after a second tick = %d, want 2", n)
	}
}

func TestDepositWatchFirstSightRecordsNothingForAReusedWalletsHistory(t *testing.T) {
	t.Parallel()
	created := clock.Real{}.Now().UTC().Add(-time.Hour)
	history := []solana.SignatureInfo{
		{Signature: "newest", Slot: 9, BlockTime: created.Add(-6 * time.Minute)},
		{Signature: "older", Slot: 8, BlockTime: created.Add(-time.Hour)},
		{Signature: "oldest", Slot: 7, BlockTime: created.Add(-24 * time.Hour)},
	}
	e := newFirstSightEnv(t, created, history)
	e.tick(t)
	if n := candidateCount(t, e.pool, ""); n != 0 {
		t.Fatalf("candidates = %d, want none for transfers before the wallet existed", n)
	}
	var high string
	if err := e.pool.QueryRow(t.Context(), `SELECT high_signature FROM deposit_watch_accounts`).
		Scan(&high); err != nil ||
		high != "newest" {
		t.Fatalf("high_signature = %q, %v; want newest", high, err)
	}
	if opening := e.opening(t); opening == nil || *opening != "7" {
		t.Fatalf("opening_micros = %v, want 7, the balance with nothing to subtract", opening)
	}
}

func TestDepositWatchFirstSightLeavesTheOpeningBalanceToExcludeRecordedCandidates(t *testing.T) {
	t.Parallel()
	created := clock.Real{}.Now().UTC().Add(-time.Hour)
	history := []solana.SignatureInfo{{Signature: "deposit", Slot: 9, BlockTime: created.Add(time.Minute)}}
	e := newFirstSightEnv(t, created, history)
	e.tick(t)
	e.assertOpeningSettlesTo(t, "2")
}

func (e *firstSightEnv) assertOpeningSettlesTo(t *testing.T, want string) {
	t.Helper()
	if opening := e.opening(t); opening != nil {
		t.Fatalf("opening_micros = %v, want NULL while a recorded candidate is unresolved", *opening)
	}
	if _, err := e.pool.Exec(t.Context(),
		`UPDATE deposit_candidates SET status = 'credited', resolved_at = now()`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(t.Context(),
		`UPDATE deposit_watch_wallets SET reconcile_due_at = now() - interval '1 minute'`); err != nil {
		t.Fatal(err)
	}
	e.ledger.set(5, 0)
	e.tick(t)
	if opening := e.opening(t); opening == nil || *opening != want {
		t.Fatalf("opening_micros = %v, want %s, the balance less the 5 the candidate credited", opening, want)
	}
}
