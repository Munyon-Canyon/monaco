package admin_test

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/app"
	adminapi "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/adminapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type stuckMoneySeed struct {
	user, cabal uuid.UUID
	withdrawal  uuid.UUID
	forward     uuid.UUID
	failed      uuid.UUID
	job         uuid.UUID
}

func (f lookupFixture) seedStuckMoney(t *testing.T, at time.Time) stuckMoneySeed {
	t.Helper()
	c := f.seedCabal(t)
	user := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "carol"}).ID.UUID()
	s := stuckMoneySeed{
		user: user, cabal: c.ID.UUID(), withdrawal: f.ids.NewV7(), forward: f.ids.NewV7(), failed: f.ids.NewV7(),
		job: f.ids.NewV7(),
	}
	f.exec(t, `INSERT INTO withdrawals (id, user_id, amount_micros, to_address, status, created_at)
		VALUES ($1, $2, 5000000, 'dest', 'created', $3)`, s.withdrawal, user, at)
	f.exec(
		t,
		`INSERT INTO withdrawals (id, user_id, amount_micros, to_address, status, created_at, completed_at, fail_code)
		VALUES ($1, $2, 5000000, 'dest', 'failed', $3, $3, 'not_sent')`,
		f.ids.NewV7(),
		user,
		at,
	)
	f.exec(
		t,
		`INSERT INTO fund_transfers (id, user_id, cabal_id, amount_micros, from_address, to_address, status, created_at)
		VALUES ($1, $2, $3, 5000000, 'a', 'b', 'created', $4)`,
		f.ids.NewV7(),
		user,
		c.ID.UUID(),
		at,
	)
	f.exec(t, `INSERT INTO cash_out_jobs (id, cabal_id, user_id, share_units, payout_micros, slice_micros, status,
		created_at, updated_at) VALUES ($1, $2, $3, 10, 5000000, 5000000, 'paying', $4, $4)`, s.job, c.ID.UUID(), user, at)
	f.exec(t, `INSERT INTO cash_out_payouts (job_id, attempt, signature, signed_tx, last_valid_block_height, status,
		created_at) VALUES ($1, 1, 'payout-sig', '\x00', 100, 'broadcast', $2)`, s.job, at)
	f.exec(t, `INSERT INTO cabal_winddowns (cabal_id, status, started_at) VALUES ($1, 'running', $2)`, c.ID.UUID(), at)
	f.exec(t, `INSERT INTO evm_deposit_forwards (id, user_id, network, source_tx, privy_execution_id, amount_micros,
		status, created_at) VALUES ($1, $2, 'base', 'tx', 'exec-1', 5000000, 'pending', $3),
		($4, $2, 'base', 'tx2', 'exec-2', 5000000, 'failed', $3)`, s.forward, user, at, s.failed)
	f.exec(t, `INSERT INTO evm_deposit_forwards (id, user_id, network, source_tx, privy_execution_id, amount_micros,
		status, solana_signature, created_at, arrived_at)
		VALUES ($1, $2, 'base', 'tx3', 'exec-3', 5000000, 'arrived', 'arrived-sig', $3, $3)`, f.ids.NewV7(), user, at)
	f.exec(
		t,
		`INSERT INTO external_deposits (id, signature, cabal_id, sender, mint, amount, source, status, detected_at)
		VALUES ($1, 'ext-1', $2, 's', 'm', 7000000, 'reconcile', 'bounce_failed', $3),
		($4, 'ext-2', $2, 's', 'm', 7000000, 'reconcile', 'held', $3)`,
		f.ids.NewV7(),
		c.ID.UUID(),
		at,
		f.ids.NewV7(),
	)
	f.exec(t, `INSERT INTO onramp_sessions (id, user_id, token_hash, status, created_at, expires_at)
		VALUES ($1, $2, sha256('a'), 'submitted', $3, $3::timestamptz + interval '1 hour'),
		($4, $2, sha256('b'), 'opened', $3, $3::timestamptz + interval '2 hours')`, f.ids.NewV7(), user, at, f.ids.NewV7())
	f.exec(t, `INSERT INTO deposit_candidates (tx_signature, wallet_address, user_id, slot, source, status, seen_at)
		VALUES ('cand-1', 'wallet', $1, 1, 'poller', 'pending', $2), ('cand-2', 'wallet', $1, 2, 'poller', 'credited', $2)`,
		user, at)
	return s
}

func TestAdminQueue_StuckMoneyListsEveryMoneyStateMachine(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	seed := f.seedStuckMoney(t, f.clock.Now())
	f.clock.Advance(10 * time.Minute)
	f.seedYoungWithdrawal(t, seed)
	var body adminapi.StuckMoneyRows
	queueBody(t, f, "/v1/admin/queues/stuck-money", &body)
	kinds := make([]string, 0, len(body.Items))
	for _, item := range body.Items {
		kinds = append(kinds, string(item.Kind))
		if item.AgeSeconds != 600 {
			t.Errorf("%s %s age = %d, want 600", item.Kind, item.Ref, item.AgeSeconds)
		}
	}
	slices.Sort(kinds)
	want := []string{
		"cash_out_job", "cash_out_payout", "deposit_candidate", "evm_forward", "evm_forward", "external_deposit",
		"fund_transfer", "onramp_session", "wind_down", "withdrawal",
	}
	if !slices.Equal(kinds, want) {
		t.Fatalf("kinds = %v, want %v (finished rows and fresh rows stay out)", kinds, want)
	}
}

func (f lookupFixture) seedYoungWithdrawal(t *testing.T, seed stuckMoneySeed) {
	t.Helper()
	f.exec(t, `INSERT INTO withdrawals (id, user_id, amount_micros, to_address, status, created_at)
		VALUES ($1, $2, 5000000, 'dest', 'created', $3)`, f.ids.NewV7(), seed.user, f.clock.Now())
}

func TestAdminQueue_StuckMoneyFiltersByKind(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	seed := f.seedStuckMoney(t, f.clock.Now())
	f.clock.Advance(10 * time.Minute)
	var forwards adminapi.StuckMoneyRows
	queueBody(t, f, "/v1/admin/queues/stuck-money?kind=evm_forward", &forwards)
	if len(forwards.Items) != 2 || forwards.Items[0].Status == forwards.Items[1].Status ||
		forwards.Items[0].UserId == nil || *forwards.Items[0].UserId != seed.user || forwards.Items[0].CabalId != nil {
		t.Fatalf("evm_forward items = %+v, want the pending and the failed forward of the member", forwards.Items)
	}
	var cabal adminapi.StuckMoneyRows
	queueBody(t, f, "/v1/admin/queues/stuck-money?kind=wind_down", &cabal)
	if len(cabal.Items) != 1 || cabal.Items[0].Ref != seed.cabal.String() || cabal.Items[0].UserId != nil ||
		*cabal.Items[0].CabalId != seed.cabal {
		t.Fatalf("wind_down items = %+v", cabal.Items)
	}
	if w := adminRequest(t, f.h, "/v1/admin/queues/stuck-money?kind=bogus", "viewer"); w.Code != http.StatusBadRequest {
		t.Fatalf("kind=bogus = %d %s, want 400", w.Code, w.Body)
	}
}

func TestAdminQueue_StuckMoneyHonoursAgeAndLimit(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	f.seedStuckMoney(t, f.clock.Now())
	f.clock.Advance(10 * time.Minute)
	var old, limited adminapi.StuckMoneyRows
	queueBody(t, f, "/v1/admin/queues/stuck-money?older_than=1h", &old)
	queueBody(t, f, "/v1/admin/queues/stuck-money?limit=3", &limited)
	if len(old.Items) != 0 || len(limited.Items) != 3 {
		t.Fatalf("older_than=1h gave %d items, limit=3 gave %d, want 0 and 3", len(old.Items), len(limited.Items))
	}
}

func TestAdminQueue_CountsCarryEveryMoneyQueueWithItsOldestAge(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	f.seedStuckMoney(t, f.clock.Now())
	f.clock.Advance(10 * time.Minute)
	var body adminapi.AdminQueues
	queueBody(t, f, "/v1/admin/queues", &body)
	got := map[string]adminapi.AdminQueue{}
	for _, q := range body.Queues {
		got[q.Name] = q
	}
	for _, kind := range money.StuckKinds() {
		q, ok := got["stuck_"+kind]
		wantCount := int64(1)
		if kind == money.StuckEVMForward {
			wantCount = 2
		}
		if !ok || q.Count != wantCount || q.Href != app.MoneyHref(kind) || q.OldestAgeSeconds == nil ||
			*q.OldestAgeSeconds != 600 {
			t.Errorf("queue stuck_%s = %+v (%v), want count %d, oldest 600s", kind, q, ok, wantCount)
		}
	}
	if got["stuck_txns"].OldestAgeSeconds != nil {
		t.Errorf("stuck_txns carries an oldest age: %+v", got["stuck_txns"])
	}
}

type cannedMoney []money.StuckSummary

func (cannedMoney) List(context.Context, string, time.Duration, int) ([]money.StuckRow, error) {
	return nil, nil
}

func (c cannedMoney) Summary(context.Context, time.Duration) ([]money.StuckSummary, error) {
	return c, nil
}

func TestQueues_OldestMoneyIntentCountsOnlyWithdrawalsTransfersAndCashOuts(t *testing.T) {
	t.Parallel()
	clk := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	ago := func(d time.Duration) time.Time { return clk.Now().Add(-d) }
	q := app.Queues{Clock: clk, Money: []money.StuckReader{cannedMoney{
		{Kind: money.StuckEVMForward, Count: 1, Oldest: ago(5 * time.Hour)},
		{Kind: money.StuckWindDown, Count: 1, Oldest: ago(4 * time.Hour)},
		{Kind: money.StuckFundTransfer, Count: 2, Oldest: ago(10 * time.Minute)},
		{Kind: money.StuckWithdrawal, Count: 1, Oldest: ago(30 * time.Minute)},
		{Kind: money.StuckCashOutJob, Count: 0, Oldest: ago(9 * time.Hour)},
	}}}
	age, err := q.OldestMoneyIntent(t.Context())
	if err != nil || age != 30*time.Minute {
		t.Fatalf(
			"OldestMoneyIntent = %v, %v, want 30m (forwards, wind-downs and empty queues are not intents)",
			age,
			err,
		)
	}
	q.Money = []money.StuckReader{cannedMoney{{Kind: money.StuckEVMForward, Count: 1, Oldest: ago(time.Hour)}}}
	if age, err := q.OldestMoneyIntent(t.Context()); err != nil || age != 0 {
		t.Fatalf("OldestMoneyIntent without intents = %v, %v, want 0", age, err)
	}
}

type downMoney struct{}

func (downMoney) List(context.Context, string, time.Duration, int) ([]money.StuckRow, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "test.List")
}

func (downMoney) Summary(context.Context, time.Duration) ([]money.StuckSummary, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "test.Summary")
}

func TestQueues_PassMoneyReaderFailuresOn(t *testing.T) {
	t.Parallel()
	q := queues(newStub())
	q.Money = []money.StuckReader{downMoney{}}
	ctx := t.Context()
	if _, err := q.Counts(ctx); !isPortDown(err) {
		t.Errorf("Counts = %v", err)
	}
	if _, err := q.StuckMoney(ctx, "", time.Minute, 1); !isPortDown(err) {
		t.Errorf("StuckMoney = %v", err)
	}
	if _, err := q.OldestMoneyIntent(ctx); !isPortDown(err) {
		t.Errorf("OldestMoneyIntent = %v", err)
	}
	h := adapters.HTTP{Queues: q}
	if _, err := h.GetStuckMoney(ctx, adminapi.GetStuckMoneyRequestObject{}); !isPortDown(err) {
		t.Errorf("GetStuckMoney = %v", err)
	}
}

func moneyGauge(t *testing.T, q app.Queues) (int64, error) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	app.ObserveMoneyIntents(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test"), q)
	var rm metricdata.ResourceMetrics
	err := reader.Collect(t.Context(), &rm)
	var got int64 = -1
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if g, ok := m.Data.(metricdata.Gauge[int64]); ok &&
				m.Name == "monaco_oldest_unresolved_money_intent_seconds" && len(g.DataPoints) == 1 {
				got = g.DataPoints[0].Value
			}
		}
	}
	return got, err
}

func TestObserveMoneyIntents_ReportsTheOldestIntentInSeconds(t *testing.T) {
	t.Parallel()
	clk := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	q := app.Queues{Clock: clk, Money: []money.StuckReader{cannedMoney{
		{Kind: money.StuckWithdrawal, Count: 1, Oldest: clk.Now().Add(-90 * time.Second)},
	}}}
	if got, err := moneyGauge(t, q); err != nil || got != 90 {
		t.Fatalf("gauge = %d, %v, want 90", got, err)
	}
	q.Money = []money.StuckReader{downMoney{}}
	if _, err := moneyGauge(t, q); err == nil {
		t.Fatal("gauge with the money readers down reported no error")
	}
}
