package ranking_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const ledgerPollerName = "ranking.weekly_ledger"

type ledgerFlow struct {
	t       *testing.T
	pool    *pgxpool.Pool
	clock   *testkit.Clock
	poller  poller.Poller
	creator uuid.UUID
	voters  []uuid.UUID
}

func sunday1659() time.Time { return time.Date(2026, 10, 25, 16, 59, 0, 0, time.UTC) }

func newLedgerFlow(t *testing.T, at time.Time) *ledgerFlow {
	t.Helper()
	clk := testkit.NewClock(at)
	var mod *ranking.Module
	var uow *db.UnitOfWork
	s := scenario.New(t, scenario.WithModules(
		func(d module.Deps) module.Module {
			d.Clock = clk
			mod = rankingOver(d)
			uow = d.UoW
			return mod
		},
		func(d module.Deps) module.Module {
			d.Config = testkit.Config()
			return treasury.New(d)
		},
	))
	f := &ledgerFlow{t: t, pool: s.DB(), clock: clk}
	if !slices.ContainsFunc(mod.Pollers(), func(p poller.Poller) bool { return p.Name() == ledgerPollerName }) {
		t.Fatalf("no poller %s registered", ledgerPollerName)
	}
	inputs := func() app.LedgerInputs {
		in := mod.LedgerInputs()
		wire := &tripwire{}
		in.Cards = fakeCards{names: map[ids.CabalID]string{}, wire: wire}
		in.Users = fakeUsers{names: map[ids.UserID]string{}, wire: wire}
		in.Market = fakeMarket{wire: wire}
		in.Trades = &fakeTrades{wire: wire}
		in.Replies = &fakeReplies{wire: wire}
		return in
	}
	f.poller = app.LedgerPoller{Builder: app.NewBuildLedgers(
		sqlc.New(f.pool), uow, inputs, mod.Ports().Cabals, clk,
	)}
	f.seed()
	return f
}

func (f *ledgerFlow) seed() {
	ctx := f.t.Context()
	cabal := testkit.NewCabal(f.t, f.pool, testkit.WithMembers(4))
	f.creator = cabal.Creator.ID.UUID()
	for _, m := range cabal.Members {
		if m.ID != cabal.Creator.ID {
			f.voters = append(f.voters, m.ID.UUID())
		}
	}
	opened := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	resolved := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	proposal := ids.Real{}.NewV7()
	if _, err := f.pool.Exec(ctx, insertLedgerCall, proposal, cabal.ID.UUID(), "buy", "TSLAx", stockMint,
		opened, opened, 500, resolved); err != nil {
		f.t.Fatalf("seed the call: %v", err)
	}
	for _, v := range f.voters {
		_, err := f.pool.Exec(ctx, insertLedgerVoter, proposal, v, cabal.ID.UUID(), "yes", opened, true)
		if err != nil {
			f.t.Fatalf("seed a voter: %v", err)
		}
	}
}

func systemCtx(ctx context.Context) context.Context {
	return auth.WithActor(ctx, auth.Actor{Kind: auth.ActorSystem, ID: "poller." + ledgerPollerName})
}

func (f *ledgerFlow) tick(ctx context.Context) poller.Report {
	f.t.Helper()
	report, err := f.poller.Tick(ctx)
	if err != nil {
		f.t.Fatalf("tick: %v", err)
	}
	return report
}

func (f *ledgerFlow) count(query string, args ...any) int {
	f.t.Helper()
	var n int
	if err := f.pool.QueryRow(f.t.Context(), query, args...).Scan(&n); err != nil {
		f.t.Fatalf("count: %v", err)
	}
	return n
}

func (f *ledgerFlow) ledgers(week string) int {
	f.t.Helper()
	return f.count(`SELECT count(*) FROM weekly_ledgers WHERE week = $1`, week)
}

func (f *ledgerFlow) skips(week string) int {
	f.t.Helper()
	return f.count(`SELECT count(*) FROM weekly_ledger_skips WHERE week = $1`, week)
}

func (f *ledgerFlow) readyEvents() int {
	f.t.Helper()
	return f.count(`SELECT count(*) FROM events WHERE type = $1`, string(events.TypeRankingLedgerReady))
}

func weekOf(at time.Time) string {
	week, _, _, _, _ := domain.LedgerWeek(app.LedgerDelivery(at))
	return week
}

func TestFlow37_BuildLedgers_OK(t *testing.T) {
	t.Parallel()
	f := newLedgerFlow(t, sunday1659())
	ctx := systemCtx(t.Context())
	week := weekOf(time.Date(2026, 10, 25, 17, 0, 0, 0, time.UTC))
	f.tick(ctx)
	if got := f.ledgers(week) + f.skips(week); got != 0 {
		t.Fatalf("rows for %s at Sunday 16:59 = %d, want 0", week, got)
	}
	if got := f.readyEvents(); got != 0 {
		t.Fatalf("ledger_ready events at Sunday 16:59 = %d, want 0", got)
	}
	f.clock.Set(time.Date(2026, 10, 25, 17, 0, 0, 0, time.UTC))
	f.tick(ctx)
	if got := f.ledgers(week); got != len(f.voters) {
		t.Fatalf("ledgers for %s at Sunday 17:00 = %d, want %d", week, got, len(f.voters))
	}
	if got := f.readyEvents(); got != len(f.voters) {
		t.Fatalf("ledger_ready events = %d, want %d", got, len(f.voters))
	}
	if got := f.skips(week); got != 1 {
		t.Fatalf("skips for %s = %d, want the creator", week, got)
	}
	f.clock.Set(time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC))
	if got := f.tick(ctx); got.Changed != 0 {
		t.Fatalf("Monday tick built %d ledgers, want none", got.Changed)
	}
	if got := f.readyEvents(); got != len(f.voters) {
		t.Fatalf("ledger_ready events after Monday = %d, want %d", got, len(f.voters))
	}
}

func TestWeeklyLedger_BatchLimit(t *testing.T) {
	t.Parallel()
	if app.LedgerBatch != 200 {
		t.Fatalf("LedgerBatch = %d, want 200", app.LedgerBatch)
	}
	f := newLedgerFlow(t, time.Date(2026, 10, 25, 17, 0, 0, 0, time.UTC))
	builder := f.poller.(app.LedgerPoller).Builder
	builder.Limit = 2
	paged := app.LedgerPoller{Builder: builder}
	week := weekOf(f.clock.Now())
	ctx := systemCtx(t.Context())
	report, err := paged.Tick(ctx)
	if err != nil || report.Scanned != 2 {
		t.Fatalf("first tick = %+v, %v; want 2 scanned", report, err)
	}
	if got := f.ledgers(week) + f.skips(week); got != 2 {
		t.Fatalf("rows after the first tick = %d, want 2", got)
	}
	f.clock.Set(time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC))
	if _, err := paged.Tick(ctx); err != nil {
		t.Fatalf("Monday tick: %v", err)
	}
	if got := f.ledgers(week) + f.skips(week); got != 4 {
		t.Fatalf("rows after the unfinished batch resumed = %d, want 4", got)
	}
	f.clock.Set(time.Date(2026, 11, 1, 17, 0, 0, 0, time.UTC))
	if _, err := paged.Tick(ctx); err != nil {
		t.Fatalf("next Sunday tick: %v", err)
	}
	if got := f.skips(weekOf(f.clock.Now())); got != 2 {
		t.Fatalf("next week's first page = %d rows, want 2", got)
	}
	if got := f.readyEvents(); got != len(f.voters) {
		t.Fatalf("ledger_ready events = %d, want %d", got, len(f.voters))
	}
}

func TestWeeklyLedger_AFailedUserDoesNotStopTheBatch(t *testing.T) {
	t.Parallel()
	f := newLedgerFlow(t, time.Date(2026, 10, 25, 17, 0, 0, 0, time.UTC))
	builder := f.poller.(app.LedgerPoller).Builder
	failing := f.voters[0]
	builder.Build = func(ctx context.Context, in app.LedgerInputs, user ids.UserID, at time.Time) (domain.Ledger, bool, error) {
		if user.UUID() == failing {
			return domain.Ledger{}, false, errs.New(errs.CodeInternal, "boom")
		}
		return app.BuildLedger(ctx, in, user, at)
	}
	report, err := app.LedgerPoller{Builder: builder}.Tick(systemCtx(t.Context()))
	if err == nil {
		t.Fatal("tick returned no error for the failed user")
	}
	week := weekOf(f.clock.Now())
	if report.Changed != len(f.voters)-1 || f.ledgers(week) != len(f.voters)-1 || f.skips(week) != 1 {
		t.Fatalf("report %+v, ledgers %d, skips %d; want %d built, 1 skipped and the failed user left pending",
			report, f.ledgers(week), f.skips(week), len(f.voters)-1)
	}
}

func TestWeeklyLedger_ARowThatAlreadyExistsEmitsNoEvent(t *testing.T) {
	t.Parallel()
	f := newLedgerFlow(t, time.Date(2026, 10, 25, 17, 0, 0, 0, time.UTC))
	builder := f.poller.(app.LedgerPoller).Builder
	builder.Build = func(
		ctx context.Context, in app.LedgerInputs, user ids.UserID, at time.Time,
	) (domain.Ledger, bool, error) {
		ledger, ok, err := app.BuildLedger(ctx, in, user, at)
		if ok {
			_, err = f.pool.Exec(ctx, `INSERT INTO weekly_ledgers
				(user_id, week, window_start, window_end, payload, built_at)
				VALUES ($1, $2, $3, $4, '{}', $3)`, user.UUID(), ledger.Week, ledger.WindowStart, ledger.WindowEnd)
		}
		return ledger, ok, err
	}
	report, err := app.LedgerPoller{Builder: builder}.Tick(systemCtx(t.Context()))
	if err != nil || report.Changed != 0 {
		t.Fatalf("tick = %+v, %v; want nothing newly built", report, err)
	}
	if got := f.readyEvents(); got != 0 {
		t.Fatalf("ledger_ready events = %d, want 0", got)
	}
}

func TestWeeklyLedger_AnEventThatCannotBeAppendedFailsTheUser(t *testing.T) {
	t.Parallel()
	f := newLedgerFlow(t, time.Date(2026, 10, 25, 17, 0, 0, 0, time.UTC))
	report, err := f.poller.Tick(t.Context())
	if err == nil || report.Changed != 0 {
		t.Fatalf("tick without an actor = %+v, %v; want failures and nothing built", report, err)
	}
	if got := f.ledgers(weekOf(f.clock.Now())); got != 0 {
		t.Fatalf("ledgers = %d, want the failed transactions rolled back", got)
	}
}
