package funding_test

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type gateRow struct {
	State                         string
	Amount                        string
	ObservedSlot, DirtyGen, Dirty int64
}

func loadGate(t *testing.T, pool *pgxpool.Pool, account chain.SolanaAddress) gateRow {
	t.Helper()
	var r gateRow
	err := pool.QueryRow(t.Context(),
		`SELECT state, last_amount::text, observed_slot, dirty_gen, dirty_slot
		FROM deposit_watch_accounts WHERE token_account = $1`, account,
	).Scan(&r.State, &r.Amount, &r.ObservedSlot, &r.DirtyGen, &r.Dirty)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func openAccount(addr, owner chain.SolanaAddress, amount uint64) solana.TokenAccountState {
	return solana.TokenAccountState{
		Address: addr, Exists: true, Program: chain.SPLProgram, Mint: testkit.USDCMint, Owner: owner,
		State: "initialized", Amount: money.NewBaseUnits(amount, 6),
	}
}

func observing(
	state solana.TokenAccountState,
	slot uint64,
) func([]chain.SolanaAddress) (uint64, []solana.TokenAccountState) {
	return func([]chain.SolanaAddress) (uint64, []solana.TokenAccountState) {
		return slot, []solana.TokenAccountState{state}
	}
}

func TestDepositWatchGateMarksTheAccountDirtyOnlyWhenItsBalanceOrExistenceChanges(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	ata := seedWatchAccount(t, pool, user)
	rpc := watchRPC{}
	p := watchFor(pool, user, now, &rpc, unlimited(), 100, 101)
	gone := solana.TokenAccountState{Address: ata}
	steps := []struct {
		name  string
		state solana.TokenAccountState
		slot  uint64
		want  gateRow
		dirty bool
	}{
		{"unchanged", openAccount(ata, user.Address, 0), 10, gateRow{"open", "0", 10, 1, 0}, false},
		{"amount changed", openAccount(ata, user.Address, 5), 11, gateRow{"open", "5", 11, 2, 11}, true},
		{"amount repeated", openAccount(ata, user.Address, 5), 12, gateRow{"open", "5", 12, 2, 11}, false},
		{"closed", gone, 13, gateRow{"closed", "0", 13, 3, 13}, true},
		{"still closed", gone, 14, gateRow{"closed", "0", 14, 3, 13}, false},
		{"reopened", openAccount(ata, user.Address, 0), 15, gateRow{"open", "0", 15, 4, 15}, true},
	}
	for _, step := range steps {
		rpc.observe = observing(step.state, step.slot)
		if _, err := p.Tick(watchActor(t)); err != nil {
			t.Fatalf("%s: Tick error = %v", step.name, err)
		}
		if got := loadGate(t, pool, ata); got != step.want {
			t.Fatalf("%s: row = %+v, want %+v", step.name, got, step.want)
		}
	}
	if got := loadAccount(t, pool, ata); got.High != "baseline" {
		t.Fatalf("high signature = %q, want the cursor kept across closure and reopening", got.High)
	}
}

func TestDepositWatchGateTreatsAnAccountThatNeverExistedAsUnchanged(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	ata := seedWatchAccount(t, pool, user)
	if _, err := pool.Exec(t.Context(), `UPDATE deposit_watch_accounts SET state = 'missing'`); err != nil {
		t.Fatal(err)
	}
	rpc := watchRPC{observe: observing(solana.TokenAccountState{Address: ata}, 4)}
	if _, err := watchFor(pool, user, now, &rpc, unlimited(), 102, 103).Tick(watchActor(t)); err != nil {
		t.Fatal(err)
	}
	if got, want := loadGate(t, pool, ata), (gateRow{"missing", "0", 4, 1, 0}); got != want {
		t.Fatalf("row = %+v, want %+v", got, want)
	}
}

func TestDepositWatchFixtureI16_SetAuthorityMakesTheAccountForeignAndItIsNeverScannedAgain(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	ata := seedWatchAccount(t, pool, user)
	rpc := watchRPC{observe: observing(openAccount(ata, "NewOwner1111111111111111111111111111111111", 9), 20)}
	p := watchFor(pool, user, now, &rpc, unlimited(), 104, 105)
	for range 2 {
		if _, err := p.Tick(watchActor(t)); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := loadGate(t, pool, ata), (gateRow{"foreign", "9", 20, 2, 20}); got != want {
		t.Fatalf("row = %+v, want %+v", got, want)
	}
	if rpc.accountsCalls != 1 || rpc.signCalls != 0 {
		t.Fatalf("accounts/signature calls = %d/%d; want the foreign row gated once and never scanned",
			rpc.accountsCalls, rpc.signCalls)
	}
	n, err := sqlc.New(pool).ApplyDepositWatchObservation(t.Context(), sqlc.ApplyDepositWatchObservationParams{
		TokenAccount: string(ata), State: "open", LastAmount: "1", ObservedSlot: 99, Dirty: true,
	})
	if err != nil || n != 0 {
		t.Fatalf("observation of a foreign row = %d, %v; want it ignored", n, err)
	}
}

func TestDepositWatchGateNeverOpensAnAccountThatIsNotAnInitializedUSDCTokenAccount(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	ata := seedWatchAccount(t, pool, user)
	if _, err := pool.Exec(t.Context(), `UPDATE deposit_watch_accounts SET state = 'missing'`); err != nil {
		t.Fatal(err)
	}
	frozen := openAccount(ata, user.Address, 7)
	frozen.State = "frozen"
	otherProgram := openAccount(ata, user.Address, 7)
	otherProgram.Program = "Token2022"
	otherMint := openAccount(ata, user.Address, 7)
	otherMint.Mint = "OtherMint"
	rpc := watchRPC{}
	p := watchFor(pool, user, now, &rpc, unlimited(), 106, 107)
	for _, state := range []solana.TokenAccountState{frozen, otherProgram, otherMint} {
		rpc.observe = observing(state, 30)
		if _, err := p.Tick(watchActor(t)); err != nil {
			t.Fatal(err)
		}
		if got, want := loadGate(t, pool, ata), (gateRow{"missing", "0", 0, 1, 0}); got != want {
			t.Fatalf("row after %+v = %+v, want it untouched", state, got)
		}
	}
}

func regressedCount(t *testing.T, reader *sdkmetric.ManualReader) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatal(err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if sum, ok := m.Data.(metricdata.Sum[int64]); ok && m.Name == "monaco_funding_watch_regressed_total" {
				return sum.DataPoints[0].Value
			}
		}
	}
	return 0
}

func TestDepositWatchGateIgnoresAnObservationOlderThanTheStoredOneAndCountsIt(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	ata := seedWatchAccount(t, pool, user)
	if _, err := pool.Exec(
		t.Context(),
		`UPDATE deposit_watch_accounts SET observed_slot = 50, last_amount = 3`,
	); err != nil {
		t.Fatal(err)
	}
	reader := sdkmetric.NewManualReader()
	counter, err := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test").
		Int64Counter(app.WatchRegressedCounter)
	if err != nil {
		t.Fatal(err)
	}
	rpc := watchRPC{pool: pool, observe: observing(openAccount(ata, user.Address, 8), 49)}
	p := app.NewDepositWatch(
		pool, db.New(pool, testkit.NewIDs(108), testkit.NewClock(now)), testkit.NewIDs(109), testkit.NewClock(now),
		fakes.NewIdentity(nil, []identity.MemberWallet{{UserID: user.ID, Address: user.Address}}),
		&rpc, testkit.USDCMint, app.DepositPollInterval, unlimited(), 480, watchTuning(), counter,
	)
	if _, err := p.Tick(watchActor(t)); err != nil {
		t.Fatal(err)
	}
	if got, want := loadGate(t, pool, ata), (gateRow{"open", "3", 50, 1, 0}); got != want {
		t.Fatalf("row = %+v, want it untouched by the older observation", got)
	}
	if got := regressedCount(t, reader); got != 1 {
		t.Fatalf("monaco_funding_watch_regressed_total = %d, want 1", got)
	}
	n, err := sqlc.New(pool).ApplyDepositWatchObservation(t.Context(), sqlc.ApplyDepositWatchObservationParams{
		TokenAccount: string(ata), State: "open", LastAmount: "8", ObservedSlot: 49, Dirty: true,
	})
	if err != nil || n != 0 {
		t.Fatalf("older observation = %d, %v; want the SQL guard to refuse it", n, err)
	}
	if got := loadGate(t, pool, ata); got.ObservedSlot != 50 {
		t.Fatalf("observed_slot = %d, want 50: it never moves back", got.ObservedSlot)
	}
}

func TestDepositWatchGateFailsTheTickWhenTheNodeAnswersBadly(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	seedWatchAccount(t, pool, user)
	rpc := watchRPC{accountsErr: errs.New(errs.CodeInternal, "test.accounts")}
	p := watchFor(pool, user, now, &rpc, unlimited(), 110, 111)
	if _, err := p.Tick(watchActor(t)); errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("Tick error = %v, want %s", err, errs.CodeRPCUnavailable)
	}
	rpc.accountsErr = nil
	rpc.observe = func([]chain.SolanaAddress) (uint64, []solana.TokenAccountState) { return 5, nil }
	if _, err := p.Tick(watchActor(t)); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("Tick error = %v, want %s for a reply shorter than the request", err, errs.CodeDecodeFailed)
	}
}

func TestDepositWatchTickGatesTenThousandIdleWalletsInOneHundredCalls(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	ctx, now := tickWithRemaining(t, 30*time.Minute)
	for _, stmt := range []string{
		`INSERT INTO deposit_watch_wallets (wallet_address, user_id, first_seen_slot, first_seen_at,
			discovery_due_at)
		SELECT 'idle-w' || i, $1, 0, now(), now() + interval '1 day' FROM generate_series(1, 10000) i`,
		`INSERT INTO deposit_watch_accounts (token_account, wallet_address, canonical, state, observed_slot,
			high_signature, recovery_due_at)
		SELECT 'idle-a' || i, 'idle-w' || i, true, 'open', 1, 'baseline', now() FROM generate_series(1, 10000) i`,
	} {
		if _, err := pool.Exec(
			t.Context(),
			strings.ReplaceAll(stmt, "$1", "'"+user.ID.UUID().String()+"'"),
		); err != nil {
			t.Fatal(err)
		}
	}
	rpc := watchRPC{observe: func(addrs []chain.SolanaAddress) (uint64, []solana.TokenAccountState) {
		states := make([]solana.TokenAccountState, len(addrs))
		for i, addr := range addrs {
			states[i] = openAccount(addr, chain.SolanaAddress(strings.Replace(string(addr), "idle-a", "idle-w", 1)), 0)
		}
		return 2, states
	}}
	p := budgetedWatch(pool, now, nil, &rpc, 480, 112, 113)
	report, err := p.Tick(ctx)
	if err != nil || report.Scanned != 10000 {
		t.Fatalf("Tick = %+v, %v; want 10000 accounts gated", report, err)
	}
	const finished = "gate=10000 gate_calls=100 dirty=0 dirty_calls=0 rotation=0 rotation_calls=0 " +
		"discovery=0 discovery_calls=0 first_sight=0 first_sight_calls=0"
	if got := stepAttrs(report); got != finished {
		t.Fatalf("step attrs = %q, want %q: every step ran and none stopped early", got, finished)
	}
	if rpc.accountsCalls != 100 || rpc.signCalls != 0 {
		t.Fatalf(
			"getMultipleAccounts/getSignaturesForAddress calls = %d/%d, want 100/0",
			rpc.accountsCalls,
			rpc.signCalls,
		)
	}
	var moved, dirty int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE observed_slot = 2),
		count(*) FILTER (WHERE dirty_gen > 0) FROM deposit_watch_accounts`).Scan(&moved, &dirty); err != nil ||
		moved != 10000 || dirty != 0 {
		t.Fatalf("observed/dirty accounts = %d/%d, %v; want 10000/0", moved, dirty, err)
	}
}
