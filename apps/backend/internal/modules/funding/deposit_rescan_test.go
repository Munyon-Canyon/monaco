package funding_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func rescanHistory() []solana.SignatureInfo {
	return []solana.SignatureInfo{
		{Signature: "sig3", Slot: 3}, {Signature: "sig2", Slot: 2}, {Signature: "sig1", Slot: 1},
	}
}

type rescanFixture struct {
	pool    *pgxpool.Pool
	user    testkit.SeededUser
	watch   *app.DepositWatch
	rescan  app.DepositRescan
	account chain.SolanaAddress
}

func newRescanFixture(t *testing.T) rescanFixture {
	t.Helper()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	history := rescanHistory()
	rpc := &watchRPC{pool: pool, signaturesFor: func(before, until chain.Signature, limit int) []solana.SignatureInfo {
		return signaturesFromHistory(history, before, until, limit)
	}}
	f := rescanFixture{
		pool: pool, user: user, watch: newDepositWatch(t, pool, user, now, rpc, 91, 92),
		rescan:  app.DepositRescan{UoW: db.New(pool, testkit.NewIDs(93), testkit.NewClock(now))},
		account: canonicalAccount(t, user.Address),
	}
	f.tick(t, 3)
	f.tick(t, 0)
	f.forget(t)
	return f
}

func (f rescanFixture) tick(t *testing.T, want int) {
	t.Helper()
	if report, err := f.watch.Tick(watchActor(t)); err != nil || report.Changed != want {
		t.Fatalf("Tick = %+v, %v, want %d candidates", report, err, want)
	}
}

func (f rescanFixture) forget(t *testing.T) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), `DELETE FROM deposit_candidates`); err != nil {
		t.Fatal(err)
	}
}

func (f rescanFixture) run(t *testing.T, from chain.Signature) {
	t.Helper()
	if accounts, err := f.rescan.Rescan(t.Context(), f.user.Address, from); err != nil || accounts != 1 {
		t.Fatalf("Rescan from %q = %d, %v, want 1 account", from, accounts, err)
	}
}

func TestDepositRescan_FromASignatureRecordsOnlyTheNewerCandidates(t *testing.T) {
	t.Parallel()
	f := newRescanFixture(t)
	f.run(t, "sig1")
	if got := loadAccount(t, f.pool, f.account); got.High != "sig1" || got.DirtyGen <= got.CleanGen {
		t.Fatalf("account after rescan = %+v, want the cursor at sig1 and the account dirty", got)
	}
	f.tick(t, 2)
	assertCandidateSignatures(t.Context(), t, f.pool, f.user.Address, "sig2", "sig3")
}

func TestDepositRescan_WithoutASignatureRecordsEveryCandidateAgainOnce(t *testing.T) {
	t.Parallel()
	f := newRescanFixture(t)
	f.run(t, "")
	if got := loadAccount(
		t,
		f.pool,
		f.account,
	); got.High != "" || got.PageBefore != "" ||
		got.DirtyGen <= got.CleanGen {
		t.Fatalf("account after a full rescan = %+v, want no cursor and the account dirty", got)
	}
	f.tick(t, 3)
	assertCandidateSignatures(t.Context(), t, f.pool, f.user.Address, "sig1", "sig2", "sig3")
	f.tick(t, 0)
}

func TestDepositRescan_RefusesAnUnknownOrEmptyWallet(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	rescan := app.DepositRescan{UoW: db.New(pool, testkit.NewIDs(94), testkit.NewClock(time.Unix(0, 0)))}
	if _, err := rescan.Rescan(t.Context(), "", ""); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Rescan of no wallet = %v, want invalid_input", err)
	}
	if _, err := rescan.Rescan(t.Context(), "UnknownWallet", ""); errs.CodeOf(err) != errs.CodeNotFound {
		t.Fatalf("Rescan of an unwatched wallet = %v, want not_found", err)
	}
}

func TestDepositRescan_ReportsAClosedDatabase(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	rescan := app.DepositRescan{UoW: db.New(closedPool(t, pool), testkit.NewIDs(95), testkit.NewClock(time.Unix(0, 0)))}
	if _, err := rescan.Rescan(t.Context(), "Wallet", ""); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Rescan on a closed pool = %v, want internal", err)
	}
}
