package funding_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const residualLogName = "funding.deposit.residual"

type residualEnv struct {
	pool   *pgxpool.Pool
	user   testkit.SeededUser
	ata    chain.SolanaAddress
	clock  *testkit.Clock
	ledger *stubLedger
	rpc    *watchRPC
	logs   *testkit.Logs
	watch  *app.DepositWatch
}

func newResidualEnv(t *testing.T, opening string, observed int64) *residualEnv {
	t.Helper()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	return residualEnvFor(t, pool, user, now, opening, observed)
}

func residualEnvFor(
	t *testing.T, pool *pgxpool.Pool, user testkit.SeededUser, now time.Time, opening string, observed int64,
) *residualEnv {
	t.Helper()
	ata := canonicalAccount(t, user.Address)
	if _, err := pool.Exec(
		t.Context(),
		`INSERT INTO deposit_watch_wallets (
			wallet_address, user_id, first_seen_slot, first_seen_at, discovery_due_at, opening_micros, reconcile_due_at)
		VALUES ($1, $2, 0, $3::timestamptz, $3::timestamptz + interval '1 day', NULLIF($4::text, '')::numeric,
			$3::timestamptz - interval '1 minute')`,
		user.Address, user.ID.UUID(), now, opening,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(
		t.Context(),
		`INSERT INTO deposit_watch_accounts (
			token_account, wallet_address, canonical, state, last_amount, observed_slot, high_signature, recovery_due_at)
		VALUES ($1, $2, true, 'open', $3, 10, 'baseline', $4::timestamptz + interval '1 day')`,
		ata, user.Address, observed, now,
	); err != nil {
		t.Fatal(err)
	}
	e := &residualEnv{
		pool: pool, user: user, ata: ata, clock: testkit.NewClock(now), ledger: &stubLedger{},
		rpc: &watchRPC{pool: pool}, logs: &testkit.Logs{},
	}
	e.watch = app.NewDepositWatch(
		pool, db.New(pool, testkit.NewIDs(410), e.clock), testkit.NewIDs(411), e.clock,
		fakes.NewIdentity(nil, []identity.MemberWallet{{UserID: user.ID, Address: user.Address}}),
		e.rpc, testkit.USDCMint, app.DepositPollInterval, unlimited(), 480, watchTuning(), noop.Int64Counter{},
		e.ledger,
	)
	return e
}

func (e *residualEnv) tick(t *testing.T) error {
	t.Helper()
	ctx := observability.WithLogger(watchActor(t), observability.NewLogger(config.Config{Env: config.EnvTest}, e.logs))
	_, err := e.watch.Tick(ctx)
	return err
}

func (e *residualEnv) nextCheck(t *testing.T) {
	t.Helper()
	e.clock.Advance(app.DepositResidualInterval + time.Minute)
}

func (e *residualEnv) alerts() int {
	return bytes.Count(e.logs.Bytes(), []byte(`"`+residualLogName+`"`))
}

func (e *residualEnv) streak(t *testing.T) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(
		t.Context(), `SELECT residual_streak FROM deposit_watch_wallets WHERE wallet_address = $1`, e.user.Address,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *residualEnv) opening(t *testing.T) string {
	t.Helper()
	var s string
	if err := e.pool.QueryRow(
		t.Context(), `SELECT coalesce(opening_micros::text, '') FROM deposit_watch_wallets WHERE wallet_address = $1`,
		e.user.Address,
	).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

type ledgerCounts struct{ deposits, events, txns, entries int }

func countLedgerTables(t *testing.T, pool *pgxpool.Pool) ledgerCounts {
	t.Helper()
	var c ledgerCounts
	if err := pool.QueryRow(
		t.Context(),
		`SELECT (SELECT count(*) FROM deposits), (SELECT count(*) FROM events),
			(SELECT count(*) FROM user_txns), (SELECT count(*) FROM user_txn_entries)`,
	).Scan(&c.deposits, &c.events, &c.txns, &c.entries); err != nil {
		t.Fatal(err)
	}
	return c
}

func int64Gauge(t *testing.T, pool *pgxpool.Pool, c clock.Clock, name string) (int64, error) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	app.ObserveDepositWatch(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test"), pool, c)
	var rm metricdata.ResourceMetrics
	err := reader.Collect(t.Context(), &rm)
	var got int64 = -1
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if g, ok := m.Data.(metricdata.Gauge[int64]); ok && m.Name == name && len(g.DataPoints) == 1 {
				got = g.DataPoints[0].Value
			}
		}
	}
	return got, err
}

func (e *residualEnv) assertResidualGauge(t *testing.T, want int64) {
	t.Helper()
	got, err := int64Gauge(t, e.pool, e.clock, "monaco_funding_deposit_residual_wallets")
	if err != nil || got != want {
		t.Fatalf("residual gauge = %d, %v; want %d", got, err, want)
	}
}

func TestDepositResidual_AlertsOnTheSecondCheckInARowAndOnlyLogs(t *testing.T) {
	t.Parallel()
	e := newResidualEnv(t, "0", 5_000_000)
	before := countLedgerTables(t, e.pool)
	if err := e.tick(t); err != nil {
		t.Fatal(err)
	}
	if e.alerts() != 0 || e.streak(t) != 1 {
		t.Fatalf("after one check alerts = %d, streak = %d; want 0 and 1", e.alerts(), e.streak(t))
	}
	e.assertResidualGauge(t, 0)
	if err := e.tick(t); err != nil || e.alerts() != 0 || e.streak(t) != 1 {
		t.Fatalf("tick inside the hour = %v, alerts %d, streak %d; want it skipped", err, e.alerts(), e.streak(t))
	}
	e.nextCheck(t)
	if err := e.tick(t); err != nil {
		t.Fatal(err)
	}
	if e.alerts() != 1 || e.streak(t) != 2 {
		t.Fatalf("after two checks alerts = %d, streak = %d; want 1 and 2", e.alerts(), e.streak(t))
	}
	e.assertResidualGauge(t, 1)
	if after := countLedgerTables(t, e.pool); after != before {
		t.Fatalf("deposits/events/user_txns/user_txn_entries = %+v, want %+v unchanged", after, before)
	}
}

func TestDepositResidual_ClearsWhenTheLedgerCatchesUp(t *testing.T) {
	t.Parallel()
	e := newResidualEnv(t, "0", 5_000_000)
	if err := e.tick(t); err != nil || e.streak(t) != 1 {
		t.Fatalf("first check = %v, streak %d; want streak 1", err, e.streak(t))
	}
	e.ledger.set(5_000_000, 0)
	e.nextCheck(t)
	if err := e.tick(t); err != nil || e.streak(t) != 0 || e.alerts() != 0 {
		t.Fatalf("explained check = %v, streak %d, alerts %d; want streak 0 and no alert", err, e.streak(t), e.alerts())
	}
}

func TestDepositResidual_NegativeResidualAlerts(t *testing.T) {
	t.Parallel()
	e := newResidualEnv(t, "0", 0)
	e.ledger.set(2_000_000, 0)
	for range 2 {
		if err := e.tick(t); err != nil {
			t.Fatal(err)
		}
		e.nextCheck(t)
	}
	if e.alerts() != 1 || !bytes.Contains(e.logs.Bytes(), []byte(`"residual_micros":"-2000000"`)) {
		t.Fatalf("alerts = %d in %s, want one with residual -2000000", e.alerts(), e.logs.Bytes())
	}
}

func TestDepositResidual_PendingLedgerRowsMakeTheCheckInconclusive(t *testing.T) {
	t.Parallel()
	e := newResidualEnv(t, "0", 5_000_000)
	e.ledger.set(0, 1)
	for range 3 {
		if err := e.tick(t); err != nil {
			t.Fatal(err)
		}
		e.nextCheck(t)
	}
	if e.alerts() != 0 || e.streak(t) != 0 {
		t.Fatalf("alerts = %d, streak = %d; want none with a pending ledger row", e.alerts(), e.streak(t))
	}
}

func TestDepositResidual_PendingCandidatesMakeTheCheckInconclusive(t *testing.T) {
	t.Parallel()
	e := newResidualEnv(t, "0", 5_000_000)
	f := candidateFixture{
		pool: e.pool, uow: db.New(e.pool, testkit.NewIDs(412), e.clock), ids: testkit.NewIDs(413), now: e.clock.Now(),
		user: e.user,
	}
	f.record(t, f.candidate())
	for range 3 {
		if err := e.tick(t); err != nil {
			t.Fatal(err)
		}
		e.nextCheck(t)
	}
	if e.alerts() != 0 || e.streak(t) != 0 {
		t.Fatalf("alerts = %d, streak = %d; want none with a pending candidate", e.alerts(), e.streak(t))
	}
}

func TestDepositResidual_ADirtyAccountMakesTheCheckInconclusive(t *testing.T) {
	t.Parallel()
	e := newResidualEnv(t, "0", 5_000_000)
	markDirty(t, e.pool, e.ata, 11)
	e.rpc.signErr = errs.New(errs.CodeRPCUnavailable, "test.minContextSlotNotReached")
	for range 3 {
		if err := e.tick(t); errs.CodeOf(err) != errs.CodeRPCUnavailable {
			t.Fatalf("tick = %v, want %s from the dirty catch-up", err, errs.CodeRPCUnavailable)
		}
		e.nextCheck(t)
	}
	if e.alerts() != 0 || e.streak(t) != 0 {
		t.Fatalf("alerts = %d, streak = %d; want none with a dirty account", e.alerts(), e.streak(t))
	}
}

func TestDepositResidual_ABaselineOpeningIsTakenFromTheFirstObservation(t *testing.T) {
	t.Parallel()
	e := newResidualEnv(t, "", 5_000_000)
	e.ledger.set(3_000_000, 0)
	if err := e.tick(t); err != nil || e.opening(t) != "2000000" || e.streak(t) != 0 {
		t.Fatalf("baseline = %v, opening %q, streak %d; want opening 2000000", err, e.opening(t), e.streak(t))
	}
	e.rpc.observe = observing(openAccount(e.ata, e.user.Address, 9_000_000), 20)
	for range 2 {
		e.nextCheck(t)
		if err := e.tick(t); err != nil {
			t.Fatal(err)
		}
	}
	if e.alerts() != 1 || e.opening(t) != "2000000" {
		t.Fatalf("alerts = %d, opening %q; want one alert and the opening kept", e.alerts(), e.opening(t))
	}
}

func TestDepositResidual_AWalletWithAnUnobservedAccountIsLeftAlone(t *testing.T) {
	t.Parallel()
	e := newResidualEnv(t, "0", 5_000_000)
	if _, err := e.pool.Exec(t.Context(), `UPDATE deposit_watch_accounts SET observed_slot = 0`); err != nil {
		t.Fatal(err)
	}
	e.rpc.observe = observing(solana.TokenAccountState{Address: e.ata, Exists: false}, 0)
	if err := e.tick(t); err != nil || e.streak(t) != 0 {
		t.Fatalf("tick = %v, streak %d; want the wallet skipped", err, e.streak(t))
	}
}

func TestDepositResidual_ALedgerFailureFailsTheStepAndChangesNothing(t *testing.T) {
	t.Parallel()
	e := newResidualEnv(t, "0", 5_000_000)
	e.ledger.err = errs.New(errs.CodeInternal, "test.ledger")
	if err := e.tick(t); errs.CodeOf(err) != errs.CodeInternal || e.streak(t) != 0 {
		t.Fatalf("tick = %v, streak %d; want %s and no change", err, e.streak(t), errs.CodeInternal)
	}
}

func TestDepositResidual_AnExpiredBudgetStopsTheStepBeforeItsFirstWallet(t *testing.T) {
	t.Parallel()
	e := newResidualEnv(t, "0", 5_000_000)
	e.rpc.observe = func([]chain.SolanaAddress) (uint64, []solana.TokenAccountState) {
		e.clock.Advance(time.Second)
		return 10, []solana.TokenAccountState{openAccount(e.ata, e.user.Address, 5_000_000)}
	}
	ctx, cancel := context.WithDeadline(watchActor(t), e.clock.Now().Add(5*time.Second+500*time.Millisecond))
	defer cancel()
	if _, err := e.watch.Tick(ctx); err != nil || e.streak(t) != 0 {
		t.Fatalf("tick = %v, streak %d; want the reconcile step stopped by the deadline", err, e.streak(t))
	}
}

func TestDepositResidual_FirstSightStoresTheOpeningNetOfTheLedger(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	ata := canonicalAccount(t, user.Address)
	rpc := &watchRPC{pool: pool, slot: 77, accounts: []solana.TokenAccountState{
		openAccount(ata, user.Address, 7_000_000), openAccount("secondAccount", user.Address, 1_000_000),
	}}
	ledger := &stubLedger{}
	ledger.set(3_000_000, 0)
	clk := testkit.NewClock(now)
	p := app.NewDepositWatch(
		pool, db.New(pool, testkit.NewIDs(414), clk), testkit.NewIDs(415), clk,
		fakes.NewIdentity(nil, []identity.MemberWallet{{UserID: user.ID, Address: user.Address}}),
		rpc, testkit.USDCMint, app.DepositPollInterval, unlimited(), 480, watchTuning(), noop.Int64Counter{}, ledger,
	)
	if _, err := p.Tick(watchActor(t)); err != nil {
		t.Fatal(err)
	}
	var opening string
	var due time.Time
	if err := pool.QueryRow(
		t.Context(),
		`SELECT opening_micros::text, reconcile_due_at FROM deposit_watch_wallets WHERE wallet_address = $1`,
		user.Address,
	).Scan(&opening, &due); err != nil {
		t.Fatal(err)
	}
	if opening != "5000000" || !due.Equal(now.Add(app.DepositResidualInterval/2)) {
		t.Fatalf("opening = %s, reconcile_due_at = %s; want 5000000 (7+1-3) and half an interval out", opening, due)
	}
}

func TestDepositResidual_FirstSightFailsWhenTheLedgerCannotBeRead(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	clk := testkit.NewClock(clock.Real{}.Now())
	ledger := &stubLedger{err: errs.New(errs.CodeInternal, "test.ledger")}
	p := app.NewDepositWatch(
		pool, db.New(pool, testkit.NewIDs(416), clk), testkit.NewIDs(417), clk,
		fakes.NewIdentity(nil, []identity.MemberWallet{{UserID: user.ID, Address: user.Address}}),
		&watchRPC{pool: pool, slot: 5}, testkit.USDCMint, app.DepositPollInterval, unlimited(), 480, watchTuning(),
		noop.Int64Counter{}, ledger,
	)
	if _, err := p.Tick(watchActor(t)); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Tick = %v, want %s", err, errs.CodeInternal)
	}
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM deposit_watch_wallets`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("wallet rows = %d, %v; want none", n, err)
	}
}

func TestDepositResidualFixtureI12_MintToIsDismissedAndRaisesAResidualAlert(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	if got := newCandidateDispatch(t, f, candidateRPC{}).deliver(t, f.candidate()); got != bus.OutcomeAck {
		t.Fatalf("delivery = %q, want ack", got)
	}
	if got := f.candidateStatus(t, f.candidate().Signature); got != "not_deposit" {
		t.Fatalf("status = %q, want not_deposit", got)
	}
	e := residualEnvFor(t, f.pool, f.user, f.now, "0", 5_000_000)
	before := countLedgerTables(t, f.pool)
	for range 2 {
		if err := e.tick(t); err != nil {
			t.Fatal(err)
		}
		e.nextCheck(t)
	}
	if e.alerts() != 1 {
		t.Fatalf("alerts = %d in %s, want one residual alert for the mintTo", e.alerts(), e.logs.Bytes())
	}
	if after := countLedgerTables(t, f.pool); after != before {
		t.Fatalf("ledger tables = %+v, want %+v unchanged", after, before)
	}
}

func TestDepositCandidatesPendingOldestGauge_ReportsTheAgeOfTheOldestPendingCandidate(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	const name = "monaco_funding_deposit_candidates_pending_oldest_seconds"
	if got, err := int64Gauge(t, f.pool, testkit.NewClock(f.now), name); err != nil || got != 0 {
		t.Fatalf("gauge with no candidates = %d, %v; want 0", got, err)
	}
	old, newer := f.candidate(), f.candidate()
	newer.Signature = "2nd" + old.Signature[3:]
	f.record(t, old, newer)
	if _, err := f.pool.Exec(
		t.Context(), `UPDATE deposit_candidates SET seen_at = $1 WHERE tx_signature = $2`,
		f.now.Add(-2*time.Hour), old.Signature,
	); err != nil {
		t.Fatal(err)
	}
	if got, err := int64Gauge(t, f.pool, testkit.NewClock(f.now), name); err != nil || got != 7200 {
		t.Fatalf("gauge = %d, %v; want 7200", got, err)
	}
	if _, err := f.pool.Exec(
		t.Context(), `UPDATE deposit_candidates SET status = 'credited' WHERE tx_signature = $1`, old.Signature,
	); err != nil {
		t.Fatal(err)
	}
	if got, err := int64Gauge(t, f.pool, testkit.NewClock(f.now), name); err != nil || got != 0 {
		t.Fatalf("gauge once the old one resolved = %d, %v; want 0 (the newer seen_at is now)", got, err)
	}
}
