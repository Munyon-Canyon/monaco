package funding_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func cleanRotationAccount(
	t *testing.T, pool *pgxpool.Pool, user testkit.SeededUser, firstSeen, high int64,
) chain.SolanaAddress {
	t.Helper()
	account := seedWatchAccount(t, pool, user)
	watchExec(t, pool, `UPDATE deposit_watch_accounts SET clean_gen = dirty_gen, high_slot = $2,
		recovery_due_at = now() - interval '1 hour' WHERE token_account = $1`, account, high)
	watchExec(t, pool, `UPDATE deposit_watch_wallets SET first_seen_slot = $2,
		discovery_due_at = now() + interval '1 day' WHERE wallet_address = $1`, user.Address, firstSeen)
	return account
}

func watchExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

func historyAt(slots ...uint64) []solana.SignatureInfo {
	history := make([]solana.SignatureInfo, len(slots))
	for i, slot := range slots {
		history[i] = solana.SignatureInfo{Signature: chain.Signature(fmt.Sprintf("sig%d", slot)), Slot: slot}
	}
	return history
}

func candidateCount(t *testing.T, pool *pgxpool.Pool, source string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM deposit_candidates WHERE $1 = '' OR source = $1`, source).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func recoveryState(t *testing.T, pool *pgxpool.Pool, account chain.SolanaAddress) (before string, due time.Time) {
	t.Helper()
	if err := pool.QueryRow(t.Context(),
		`SELECT coalesce(recovery_before, ''), recovery_due_at FROM deposit_watch_accounts WHERE token_account = $1`,
		account).Scan(&before, &due); err != nil {
		t.Fatal(err)
	}
	return before, due
}

func TestDepositWatchFixtureI1_EqualAndOppositeFlowIsCaughtByRotation(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	account := cleanRotationAccount(t, pool, user, 0, 100)
	rpc := watchRPC{signaturesFor: func(before, until chain.Signature, limit int) []solana.SignatureInfo {
		return signaturesFromHistory(historyAt(95, 90), before, until, limit)
	}}
	report, err := watchFor(pool, user, now, &rpc, unlimited(), 120, 121).Tick(watchActor(t))
	if err != nil || report.Changed != 2 {
		t.Fatalf("Tick = %+v, %v; want the gate to see no change and rotation to record both transfers", report, err)
	}
	if rpc.accountsCalls != 1 || loadAccount(t, pool, account).DirtyGen != 1 {
		t.Fatalf("gate calls = %d, dirty_gen = %d; want one call that marked nothing", rpc.accountsCalls,
			loadAccount(t, pool, account).DirtyGen)
	}
	if got := candidateCount(t, pool, "recovery"); got != 2 {
		t.Fatalf("recovery candidates = %d, want 2", got)
	}
	before, due := recoveryState(t, pool, account)
	if before != "" || !due.Equal(now.Add(6*time.Hour)) {
		t.Fatalf("recovery_before/due = %q/%s, want it cleared and due in six hours", before, due)
	}
}

func TestDepositWatchRotationStopsAtTheRecoveryFloor(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name            string
		firstSeen, high int64
		want            []string
	}{
		{"recovery window", 0, 5000, []string{"sig4500", "sig4900"}},
		{"first sight", 4600, 5000, []string{"sig4900"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := testkit.DB(t)
			user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
			now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
			cleanRotationAccount(t, pool, user, tc.firstSeen, tc.high)
			rpc := watchRPC{signaturesFor: func(before, until chain.Signature, limit int) []solana.SignatureInfo {
				return signaturesFromHistory(historyAt(4900, 4500, 3999, 3000), before, until, limit)
			}}
			if _, err := watchFor(pool, user, now, &rpc, unlimited(), 122, 123).Tick(watchActor(t)); err != nil {
				t.Fatal(err)
			}
			assertCandidateSignatures(t.Context(), t, pool, user.Address, tc.want...)
			if rpc.signCalls != 1 || rpc.untils[0] != "" {
				t.Fatalf("signature calls = %d, untils = %q; want one call without Until", rpc.signCalls, rpc.untils)
			}
		})
	}
}

func TestDepositWatchRotationPagesAndResumesWhereTheCallBudgetRanOut(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	account := cleanRotationAccount(t, pool, user, 0, 2000)
	slots := make([]uint64, 1500)
	for i := range slots {
		slots[i] = uint64(2000 - i)
	}
	history := historyAt(slots...)
	rpc := watchRPC{signaturesFor: func(before, until chain.Signature, limit int) []solana.SignatureInfo {
		return signaturesFromHistory(history, before, until, limit)
	}}
	wallets := []identity.MemberWallet{{UserID: user.ID, Address: user.Address}}
	if _, err := budgetedWatch(pool, now, wallets, &rpc, 2, 124, 125).Tick(watchActor(t)); err != nil {
		t.Fatal(err)
	}
	before, _ := recoveryState(t, pool, account)
	if before != "sig1001" || candidateCount(t, pool, "recovery") != 1000 {
		t.Fatalf("recovery_before = %q with %d candidates; want a checkpoint after the first page",
			before, candidateCount(t, pool, "recovery"))
	}
	if _, err := budgetedWatch(pool, now, wallets, &rpc, 3, 126, 127).Tick(watchActor(t)); err != nil {
		t.Fatal(err)
	}
	before, due := recoveryState(t, pool, account)
	if got := candidateCount(t, pool, "recovery"); got != 1001 || before != "" || !due.Equal(now.Add(6*time.Hour)) {
		t.Fatalf("candidates/before/due = %d/%q/%s; want the floor at slot 1000 to end the rotation", got, before, due)
	}
}

func TestDepositWatchFixtureI8_ProviderOmittedSignatureIsRecordedByRecovery(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	account := cleanRotationAccount(t, pool, user, 0, 100)
	history := historyAt(30, 10)
	rpc := watchRPC{signaturesFor: func(before, until chain.Signature, limit int) []solana.SignatureInfo {
		return signaturesFromHistory(history, before, until, limit)
	}}
	if _, err := watchFor(pool, user, now, &rpc, unlimited(), 128, 129).Tick(watchActor(t)); err != nil {
		t.Fatal(err)
	}
	assertCandidateSignatures(t.Context(), t, pool, user.Address, "sig10", "sig30")
	watchExec(t, pool, `UPDATE deposit_watch_accounts SET recovery_due_at = now() - interval '1 hour'
		WHERE token_account = $1`, account)
	history = historyAt(30, 20, 10)
	report, err := watchFor(pool, user, now, &rpc, unlimited(), 130, 131).Tick(watchActor(t))
	if err != nil || report.Changed != 1 {
		t.Fatalf("second Tick = %+v, %v; want only the omitted signature recorded", report, err)
	}
	assertCandidateSignatures(t.Context(), t, pool, user.Address, "sig10", "sig20", "sig30")
	var source string
	if err := pool.QueryRow(t.Context(),
		`SELECT source FROM deposit_candidates WHERE tx_signature = 'sig20'`).Scan(&source); err != nil ||
		source != "recovery" {
		t.Fatalf("source = %q, %v; want recovery", source, err)
	}
}

func TestDepositWatchFixtureI2_OneTransactionPayingTwoAccountsCreditsTheWalletSumOnce(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		both  bool
		calls int
	}{
		{"found from both accounts", true, 2},
		{"found from the non-canonical account alone", false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := testkit.DB(t)
			user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
			now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
			canonical := cleanRotationAccount(t, pool, user, 0, 100)
			other := chain.SolanaAddress("Other111111111111111111111111111111111111111")
			watchExec(t, pool, `INSERT INTO deposit_watch_accounts (token_account, wallet_address, canonical, state,
				clean_gen, dirty_gen, recovery_due_at) VALUES ($1, $2, false, 'open', 0, 0, now() - interval '1 hour')`,
				other, user.Address)
			if !tc.both {
				watchExec(t, pool, `UPDATE deposit_watch_accounts SET recovery_due_at = now() + interval '1 day'
					WHERE token_account = $1`, canonical)
			}
			rpc := watchRPC{signaturesFor: func(before, until chain.Signature, limit int) []solana.SignatureInfo {
				return signaturesFromHistory(historyAt(50), before, until, limit)
			}}
			report, err := watchFor(pool, user, now, &rpc, unlimited(), 132, 133).Tick(watchActor(t))
			if err != nil || report.Changed != 1 || rpc.signCalls != tc.calls {
				t.Fatalf("Tick = %+v, %v with %d signature calls; want one candidate from %d call(s)",
					report, err, rpc.signCalls, tc.calls)
			}
			assertCandidateSignatures(t.Context(), t, pool, user.Address, "sig50")
			creditWalletSum(t, pool, user)
		})
	}
}

func creditWalletSum(t *testing.T, pool *pgxpool.Pool, user testkit.SeededUser) {
	t.Helper()
	var slot int64
	if err := pool.QueryRow(t.Context(),
		`SELECT slot FROM deposit_candidates WHERE tx_signature = 'sig50'`).Scan(&slot); err != nil {
		t.Fatal(err)
	}
	uow := db.New(pool, testkit.NewIDs(134), clock.Real{})
	g := testkit.NewIDs(135)
	usdc := chain.Mint{Address: testkit.USDCMint, Decimals: 6}
	resolver := app.NewDepositCandidateResolver(candidateRPC{transfers: []solana.Transfer{
		{Mint: usdc, Net: money.NewBaseUnits(5_000_000, 6)}, {Mint: usdc, Net: money.NewBaseUnits(7_000_000, 6)},
	}}, testkit.USDCMint, nil, app.NewCreditDepositHandler(uow, &hints{}), g)
	e := events.DepositCandidateSeen{
		V: 1, CandidateID: g.NewV7(), UserID: user.ID.UUID(), WalletAddress: user.Address, TxSignature: "sig50",
		Slot: slot, Source: "recovery",
	}
	resolution, err := resolver.Fetch(t.Context(), e)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		err := uow.Do(watchActor(t), func(ctx context.Context, tx db.Tx) error {
			return resolver.Apply(ctx, tx, e, resolution, clock.Real{}.Now())
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	var deposits int
	var amount string
	if err := pool.QueryRow(t.Context(), `SELECT count(*), coalesce(sum(amount_micros), 0)::text FROM deposits
		WHERE tx_signature = 'sig50' AND wallet_address = $1`, user.Address).Scan(&deposits, &amount); err != nil ||
		deposits != 1 || amount != "12000000" {
		t.Fatalf("deposits/amount = %d/%s, %v; want one credit for the wallet-wide 12 USDC", deposits, amount, err)
	}
}

func TestDepositWatchDiscoveryAddsAnUnknownAccountDirtyAndLeavesKnownOnesToTheGate(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	canonical := cleanRotationAccount(t, pool, user, 100, 100)
	watchExec(t, pool, `UPDATE deposit_watch_accounts SET recovery_due_at = now() + interval '1 day'`)
	watchExec(t, pool, `UPDATE deposit_watch_wallets SET discovery_due_at = NULL`)
	other := chain.SolanaAddress("Other111111111111111111111111111111111111111")
	rpc := watchRPC{
		slot:     200,
		accounts: []solana.TokenAccountState{{Address: other, Exists: true, Amount: money.NewBaseUnits(5, 6)}},
		signaturesFor: func(before, until chain.Signature, limit int) []solana.SignatureInfo {
			return signaturesFromHistory(historyAt(250, 150, 99, 50), before, until, limit)
		},
	}
	ctx := watchActor(t)
	p := watchFor(pool, user, now, &rpc, unlimited(), 136, 137)
	if _, err := p.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	added := loadAccount(t, pool, other)
	if added.High != "" || added.DirtyGen != 1 || added.CleanGen != 0 {
		t.Fatalf("new account = %+v; want it open and dirty with no high signature", added)
	}
	if kept := loadAccount(t, pool, canonical); kept.DirtyGen != kept.CleanGen {
		t.Fatalf("canonical account = %+v; want it left to the gate", kept)
	}
	var due time.Time
	if err := pool.QueryRow(t.Context(), `SELECT discovery_due_at FROM deposit_watch_wallets`).Scan(&due); err != nil ||
		!due.Equal(now.Add(6*time.Hour)) {
		t.Fatalf("discovery_due_at = %s, %v; want six hours out", due, err)
	}
	if report, err := p.Tick(ctx); err != nil || report.Changed != 2 {
		t.Fatalf("second Tick = %+v, %v; want the catch-up to stop at first_seen_slot 100", report, err)
	}
	assertCandidateSignatures(ctx, t, pool, user.Address, "sig150", "sig250")
	if got := loadAccount(t, pool, other); got.CleanGen != got.DirtyGen || got.High != "sig250" {
		t.Fatalf("new account after catch-up = %+v; want it clean at the newest signature", got)
	}
}

func TestDepositWatchFixtureI10_UnknownAccountOpenedAndClosedBetweenDiscoveryPassesIsTheDocumentedMiss(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	canonical := cleanRotationAccount(t, pool, user, 0, 100)
	watchExec(t, pool, `UPDATE deposit_watch_accounts SET recovery_due_at = now() + interval '1 day'`)
	watchExec(t, pool, `UPDATE deposit_watch_wallets SET discovery_due_at = now() - interval '1 hour'`)
	rpc := watchRPC{
		slot:     300,
		accounts: []solana.TokenAccountState{{Address: canonical, Exists: true}},
		signaturesFor: func(before, until chain.Signature, limit int) []solana.SignatureInfo {
			return signaturesFromHistory(historyAt(250), before, until, limit)
		},
	}
	if _, err := watchFor(pool, user, now, &rpc, unlimited(), 138, 139).Tick(watchActor(t)); err != nil {
		t.Fatal(err)
	}
	var accounts int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM deposit_watch_accounts`).Scan(&accounts); err != nil ||
		accounts != 1 || candidateCount(t, pool, "") != 0 || rpc.signCalls != 0 {
		t.Fatalf("accounts/candidates/signature calls = %d/%d/%d, %v; want 1/0/0",
			accounts, candidateCount(t, pool, ""), rpc.signCalls, err)
	}
}

func TestDepositWatchDiscoveryRespectsTheCallBudget(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	for _, wallet := range []string{"w1", "w2"} {
		watchExec(t, pool, `INSERT INTO deposit_watch_wallets (wallet_address, user_id, first_seen_slot, first_seen_at)
			VALUES ($1, $2, 0, now())`, wallet, user.ID.UUID())
	}
	report, err := budgetedWatch(pool, now, nil, &watchRPC{}, 1, 140, 141).Tick(watchActor(t))
	if err != nil || stepAttrs(report) != "gate=0 gate_calls=0 dirty=0 dirty_calls=0 rotation=0 rotation_calls=0 "+
		"discovery=1 discovery_calls=1" {
		t.Fatalf("Tick = %q, %v; want one wallet discovered and the tick stopped", stepAttrs(report), err)
	}
	var pending int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM deposit_watch_wallets WHERE discovery_due_at IS NULL`).Scan(&pending); err != nil ||
		pending != 1 {
		t.Fatalf("wallets left undiscovered = %d, %v; want 1", pending, err)
	}
}

func TestDepositWatchFirstSightSpreadsTheFirstRotationAndDiscovery(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	rpc := watchRPC{slot: 5}
	if _, err := watchFor(pool, user, now, &rpc, unlimited(), 142, 143).Tick(watchActor(t)); err != nil {
		t.Fatal(err)
	}
	var recovery, discovery time.Time
	if err := pool.QueryRow(t.Context(), `SELECT a.recovery_due_at, w.discovery_due_at FROM deposit_watch_accounts a
		JOIN deposit_watch_wallets w USING (wallet_address)`).Scan(&recovery, &discovery); err != nil ||
		!recovery.Equal(now.Add(3*time.Hour)) || !discovery.Equal(now.Add(3*time.Hour)) {
		t.Fatalf("recovery/discovery due = %s/%s, %v; want half a period out", recovery, discovery, err)
	}
}
