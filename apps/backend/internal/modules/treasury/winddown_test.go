package treasury_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type deposit struct{ micros, shares int64 }

type windDownRig struct {
	f       fixture
	cabal   ids.CabalID
	members []ids.UserID
	paused  *atomic.Bool
	pot     *atomic.Pointer[error]
	empty   *atomic.Bool
	wd      *app.WindDown
	poller  adapters.WindDownPoller
}

func newWindDownRig(t *testing.T, deposits ...deposit) windDownRig {
	t.Helper()
	return newWindDownRigOn(t, newFixture(t), deposits...)
}

func newWindDownRigOn(t *testing.T, f fixture, deposits ...deposit) windDownRig {
	t.Helper()
	r := windDownRig{
		f:      f,
		cabal:  f.cabal(t),
		paused: &atomic.Bool{},
		pot:    &atomic.Pointer[error]{},
		empty:  &atomic.Bool{},
	}
	for _, d := range deposits {
		user := f.user(t)
		u, c, err := f.fund(user, r.cabal, d.micros, d.shares, "settled")
		if err != nil || f.postPair(u, c) != nil {
			t.Fatalf("fund = %v", err)
		}
		r.members = append(r.members, user)
	}
	pauses := app.CashOutPauseFunc(func(context.Context, ids.CabalID) (app.CashOutPause, error) {
		return app.CashOutPause{Paused: r.paused.Load()}, nil
	})
	values := failingPot{PositionsReader: newQueries(f), fail: r.pot, empty: r.empty}
	handler := app.NewCashOutHandler(f.uow, f.ledger, values, pauses, f.clock, f.ids, f.pool, &hints{})
	r.wd = app.NewWindDown(f.uow, handler, f.pool, f.clock)
	r.poller = adapters.WindDownPoller{WindDown: r.wd}
	return r
}

type failingPot struct {
	port.PositionsReader
	fail  *atomic.Pointer[error]
	empty *atomic.Bool
}

func (p failingPot) PotValue(ctx context.Context, cabal ids.CabalID) (money.Micros, error) {
	if err := p.fail.Load(); err != nil {
		return money.Micros{}, *err
	}
	if p.empty.Load() {
		return money.Micros{}, nil
	}
	return p.PositionsReader.PotValue(ctx, cabal)
}

func (r windDownRig) system() context.Context {
	return observability.WithActor(r.f.ctx(), "system:poller.treasury.winddown")
}

func (r windDownRig) start() error {
	return r.f.uow.Do(r.system(), r.startIn)
}

func (r windDownRig) startIn(ctx context.Context, tx db.Tx) error {
	return r.wd.Start(ctx, tx, events.CabalBanned{V: 1, CabalID: r.cabal.UUID()}, r.f.clock.Now())
}

func (r windDownRig) scalar(t *testing.T, query string, args ...any) (n int64) {
	t.Helper()
	if err := r.f.pool.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r windDownRig) jobs(t *testing.T) int64 {
	t.Helper()
	return r.scalar(t, `SELECT count(*) FROM cash_out_jobs WHERE cabal_id = $1 AND cause = 'wind_down'`, r.cabal.UUID())
}

func (r windDownRig) held(t *testing.T) int64 {
	t.Helper()
	return r.scalar(
		t,
		`SELECT coalesce(sum(share_units), 0)::bigint FROM user_positions WHERE cabal_id = $1`,
		r.cabal.UUID(),
	)
}

func (r windDownRig) attempts(t *testing.T) int64 {
	t.Helper()
	return r.scalar(t, `SELECT attempts FROM cabal_winddowns WHERE cabal_id = $1`, r.cabal.UUID())
}

func (r windDownRig) wound(t *testing.T) (payload map[string]any) {
	t.Helper()
	var raw []byte
	err := r.f.pool.QueryRow(t.Context(), `SELECT payload FROM events WHERE type = 'cabal.wound_down'`).Scan(&raw)
	if err != nil || json.Unmarshal(raw, &payload) != nil {
		t.Fatalf("cabal.wound_down = %v", err)
	}
	return payload
}

func (r windDownRig) finish(t *testing.T) {
	t.Helper()
	_, err := r.f.pool.Exec(t.Context(), `UPDATE cash_out_jobs SET status = 'completed' WHERE cause = 'wind_down'`)
	if err != nil {
		t.Fatal(err)
	}
}

func (r windDownRig) tick(t *testing.T) (scanned, changed int) {
	t.Helper()
	report, err := r.poller.Tick(r.system())
	if err != nil {
		t.Fatalf("Tick = %v", err)
	}
	return report.Scanned, report.Changed
}

func thirds() []deposit { return []deposit{{60_000_000, 60}, {30_000_000, 30}, {10_000_000, 10}} }

func TestWindDown_Start_CashesOutEveryHolderOnceAndKeepsTheLedgerBalanced(t *testing.T) {
	t.Parallel()
	r := newWindDownRig(t, thirds()...)
	for range 2 {
		if err := r.start(); err != nil {
			t.Fatal(err)
		}
	}
	started := r.scalar(
		t,
		`SELECT count(*) FROM events WHERE type = 'cashout.started' AND payload->>'cause' = 'wind_down'`,
	)
	if r.jobs(t) != 3 || started != 3 || r.held(t) != 0 || r.attempts(t) != 0 {
		t.Fatalf("jobs %d, started events %d, shares held %d, want 3, 3 and none", r.jobs(t), started, r.held(t))
	}
	if paid := r.scalar(t, `SELECT sum(payout_micros)::bigint FROM cash_out_jobs`); paid != 100_000_000 {
		t.Fatalf("payouts = %d, want the whole pot", paid)
	}
	if drift := r.f.drift(t); len(drift) != 0 {
		t.Fatalf("ledger drift = %v", drift)
	}
}

func TestWindDown_PaysAMemberUnderTheMinimum(t *testing.T) {
	t.Parallel()
	r := newWindDownRig(t, deposit{99_000_000, 1_000_000}, deposit{1_000_000, 1})
	if err := r.start(); err != nil {
		t.Fatal(err)
	}
	if r.jobs(t) != 2 || r.held(t) != 0 {
		t.Fatalf("jobs %d, shares held %d, want both members cashed out", r.jobs(t), r.held(t))
	}
	dust := r.scalar(t, `SELECT min(payout_micros)::bigint FROM cash_out_jobs WHERE cabal_id = $1`, r.cabal.UUID())
	paid := r.scalar(t, `SELECT sum(payout_micros)::bigint FROM cash_out_jobs WHERE cabal_id = $1`, r.cabal.UUID())
	if dust <= 0 || dust >= 100_000 || paid != 100_000_000 {
		t.Fatalf(
			"smallest payout %d, total %d, want a positive slice under 0.10 USDC and the whole pot paid",
			dust,
			paid,
		)
	}
	if started := r.scalar(t, `SELECT count(*) FROM events WHERE type = 'cashout.started'`); started != 2 {
		t.Fatalf("started events = %d, want a transfer for the dust member too", started)
	}
	r.finish(t)
	if _, changed := r.tick(t); changed != 1 {
		t.Fatal("tick did not complete the wind-down")
	}
	if wound := r.wound(t); wound["members_paid"] != 2.0 || wound["usdc_returned_micros"] != "100000000" {
		t.Fatalf("cabal.wound_down = %v, want 2 members and the whole pot", wound)
	}
	if drift := r.f.drift(t); len(drift) != 0 {
		t.Fatalf("ledger drift = %v", drift)
	}
}

func TestWindDown_ZeroPotCompletesWithWoundDown(t *testing.T) {
	t.Parallel()
	r := newWindDownRig(t, thirds()...)
	r.empty.Store(true)
	if err := r.start(); err != nil {
		t.Fatal(err)
	}
	done := r.scalar(
		t,
		`SELECT count(*) FROM cash_out_jobs WHERE cabal_id = $1 AND status = 'completed' AND payout_micros = 0`,
		r.cabal.UUID(),
	)
	if r.jobs(t) != 3 || done != 3 || r.held(t) != 0 {
		t.Fatalf("jobs %d, done at once %d, shares held %d, want 3, 3 and none", r.jobs(t), done, r.held(t))
	}
	if started := r.scalar(t, `SELECT count(*) FROM events WHERE type = 'cashout.started'`); started != 0 {
		t.Fatalf("started events = %d, want none: nothing is transferred", started)
	}
	if got := r.scalar(
		t,
		`SELECT count(*) FROM events WHERE type = 'cashout.completed' AND payload->>'cause' = 'wind_down'`,
	); got != 3 {
		t.Fatalf("cashout.completed events = %d, want one per member", got)
	}
	if _, changed := r.tick(t); changed != 1 {
		t.Fatal("tick did not complete the wind-down")
	}
	if wound := r.wound(t); wound["members_paid"] != 0.0 || wound["usdc_returned_micros"] != "0" {
		t.Fatalf("cabal.wound_down = %v, want no members paid and nothing returned", wound)
	}
	if drift := r.f.drift(t); len(drift) != 0 {
		t.Fatalf("ledger drift = %v", drift)
	}
}

func (r windDownRig) reissue(t *testing.T, member ids.UserID, units int64) {
	t.Helper()
	r.f.exec(t, `UPDATE cash_out_jobs SET status = 'failed' WHERE cause = 'wind_down' AND status <> 'failed'`)
	r.f.exec(t, fmt.Sprintf(`UPDATE user_positions SET share_units = %d WHERE cabal_id = '%s' AND user_id = '%s'`,
		units, r.cabal, member))
}

func TestWindDown_Poller_CountsReissuesThenAlertsWhenStuck(t *testing.T) {
	t.Parallel()
	r := newWindDownRig(t, thirds()...)
	if err := r.start(); err != nil {
		t.Fatal(err)
	}
	for attempt := range int64(app.WindDownMaxAttempts) {
		r.reissue(t, r.members[2], 10)
		if scanned, changed := r.tick(t); scanned != 1 || changed != 1 || r.attempts(t) != attempt+1 {
			t.Fatalf("re-issue %d: scanned %d changed %d attempts %d", attempt+1, scanned, changed, r.attempts(t))
		}
	}
	r.reissue(t, r.members[2], 10)
	if _, err := r.poller.Tick(r.system()); err == nil || r.attempts(t) != app.WindDownMaxAttempts {
		t.Fatalf("tick past the cap = %v with %d attempts, want an error and no new attempt", err, r.attempts(t))
	}
	if !bytes.Contains(r.f.logs.Bytes(), []byte("treasury.winddown_stuck")) {
		t.Fatal("no treasury.winddown_stuck alert logged")
	}
}

func TestWindDown_Poller_DeferredTicksAreNotAttempts(t *testing.T) {
	t.Parallel()
	r := newWindDownRig(t, thirds()...)
	r.paused.Store(true)
	if err := r.start(); err != nil {
		t.Fatal(err)
	}
	for range 2 * app.WindDownMaxAttempts {
		if _, changed := r.tick(t); changed != 0 || r.attempts(t) != 0 {
			t.Fatalf("a deferred tick changed %d and counted %d attempts, want neither", changed, r.attempts(t))
		}
	}
	r.paused.Store(false)
	if _, changed := r.tick(t); changed != 1 || r.jobs(t) != 3 || r.held(t) != 0 || r.attempts(t) != 1 {
		t.Fatalf(
			"tick once the cause cleared: changed %d, %d jobs, %d held, %d attempts",
			changed,
			r.jobs(t),
			r.held(t),
			r.attempts(t),
		)
	}
}

func TestWindDown_Poller_AlertsWhenARunningWindDownIsADayOld(t *testing.T) {
	t.Parallel()
	r := newWindDownRig(t, thirds()...)
	r.paused.Store(true)
	if err := r.start(); err != nil {
		t.Fatal(err)
	}
	r.tick(t)
	if bytes.Contains(r.f.logs.Bytes(), []byte("treasury.winddown_stuck")) {
		t.Fatal("alerted on a fresh wind-down")
	}
	r.f.clock.Advance(app.WindDownMaxAge + time.Minute)
	if _, err := r.poller.Tick(
		r.system(),
	); err != nil ||
		!bytes.Contains(r.f.logs.Bytes(), []byte("treasury.winddown_stuck")) {
		t.Fatalf("tick on a day-old wind-down = %v, want the stuck alert and no error", err)
	}
}

func TestWindDown_Poller_StartsMembersWhoseCashOutWasHeldBack(t *testing.T) {
	t.Parallel()
	r := newWindDownRig(t, thirds()...)
	r.paused.Store(true)
	if err := r.start(); err != nil || r.jobs(t) != 0 || r.held(t) != 100 {
		t.Fatalf("start while paused = %v with %d jobs, %d shares held", err, r.jobs(t), r.held(t))
	}
	r.paused.Store(false)
	if _, changed := r.tick(t); changed != 1 || r.jobs(t) != 3 || r.held(t) != 0 || r.attempts(t) != 1 {
		t.Fatalf("tick: changed %d, %d jobs, %d held, %d attempts", changed, r.jobs(t), r.held(t), r.attempts(t))
	}
}

func TestWindDown_Poller_CompletesOnceEveryPayoutEnded(t *testing.T) {
	t.Parallel()
	r := newWindDownRig(t, thirds()...)
	if err := r.start(); err != nil {
		t.Fatal(err)
	}
	if _, changed := r.tick(t); changed != 0 {
		t.Fatal("tick completed the wind-down while payouts were still in flight")
	}
	r.finish(t)
	if _, changed := r.tick(t); changed != 1 {
		t.Fatal("tick did not complete the wind-down once every payout ended")
	}
	wound := r.wound(t)
	if wound["members_paid"] != 3.0 || wound["usdc_returned_micros"] != "100000000" {
		t.Fatalf("cabal.wound_down = %v, want 3 members and 100000000 micros", wound)
	}
	if scanned, _ := r.tick(t); scanned != 0 {
		t.Fatalf("tick after completion scanned %d, want nothing", scanned)
	}
}

func TestWindDown_StoreFailures(t *testing.T) {
	t.Parallel()
	type step func(r windDownRig) error
	tick := func(r windDownRig) error { _, err := r.poller.Tick(r.system()); return err }
	cases := map[string]struct {
		arrange func(t *testing.T, r windDownRig)
		table   string
		run     step
	}{
		"start cannot record the wind-down": {nil, "cabal_winddowns", windDownRig.start},
		"start cannot list holders":         {nil, "user_positions", windDownRig.start},
		"tick cannot list wind-downs": {func(t *testing.T, r windDownRig) {
			t.Helper()
			if err := r.start(); err != nil {
				t.Fatal(err)
			}
		}, "cabal_winddowns", tick},
		"tick cannot list holders": {func(t *testing.T, r windDownRig) {
			t.Helper()
			if err := r.start(); err != nil {
				t.Fatal(err)
			}
		}, "user_positions", tick},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newWindDownRig(t, thirds()...)
			if c.arrange != nil {
				c.arrange(t, r)
			}
			if _, err := r.f.pool.Exec(t.Context(), `ALTER TABLE `+c.table+` RENAME TO `+c.table+`_gone`); err != nil {
				t.Fatal(err)
			}
			if err := c.run(r); err == nil {
				t.Fatal("error = nil, want the store failure")
			}
		})
	}
}

func TestWindDown_Start_KeepsTheRunningRowWhenAMemberFails(t *testing.T) {
	t.Parallel()
	r := newWindDownRig(t, thirds()...)
	broken := errs.New(errs.CodeUpstreamUnavailable, "test")
	r.pot.Store(&broken)
	if err := r.start(); err != nil || r.scalar(t, `SELECT count(*) FROM cabal_winddowns`) != 1 || r.jobs(t) != 0 {
		t.Fatalf("start with the pot unreadable = %v, want the running row kept and no job", err)
	}
	if _, err := r.poller.Tick(r.system()); errs.CodeOf(err) != errs.CodeUpstreamUnavailable || r.attempts(t) != 0 {
		t.Fatalf(
			"tick with the pot unreadable = %v with %d attempts, want upstream_unavailable and none",
			err,
			r.attempts(t),
		)
	}
	r.pot.Store(nil)
	if _, changed := r.tick(t); changed != 1 || r.jobs(t) != 3 || r.held(t) != 0 {
		t.Fatalf(
			"tick once the pot reads: changed %d, %d jobs, %d held, want the poller to cash everyone out",
			changed,
			r.jobs(t),
			r.held(t),
		)
	}
}

func TestWindDown_Poller_FailsWhenItCannotCountAnAttemptOrComplete(t *testing.T) {
	t.Parallel()
	r := newWindDownRig(t, thirds()...)
	r.paused.Store(true)
	if err := r.start(); err != nil {
		t.Fatal(err)
	}
	r.paused.Store(false)
	r.f.exec(t, `CREATE FUNCTION refuse() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'no'; END $$`)
	r.f.exec(t, `CREATE TRIGGER refuse BEFORE UPDATE ON cabal_winddowns FOR EACH ROW EXECUTE FUNCTION refuse()`)
	if _, err := r.poller.Tick(r.system()); err == nil {
		t.Fatal("tick that cannot count its attempt = nil error")
	}
	r.finish(t)
	if _, err := r.poller.Tick(r.system()); err == nil {
		t.Fatal("tick that cannot complete = nil error")
	}
}

func TestWindDown_Metadata(t *testing.T) {
	t.Parallel()
	r := newWindDownRig(t)
	if r.poller.Name() != "treasury.winddown" || r.poller.Interval().Minutes() != 2 {
		t.Fatalf("poller = %s every %s, want treasury.winddown every 2m", r.poller.Name(), r.poller.Interval())
	}
}

func (f fixture) exec(t *testing.T, query string) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), query); err != nil {
		t.Fatal(err)
	}
}
