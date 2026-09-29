package replay_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/system"
	"github.com/monaco/monaco/apps/backend/internal/modules/system/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/system/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/tools/ops/replay"
)

type flow00 struct {
	pool    *pgxpool.Pool
	clock   *testkit.Clock
	uow     *db.UnitOfWork
	pinned  *replay.Clock
	echoUoW *db.UnitOfWork
	echo    bus.HandlerSpec
	events  []uuid.UUID
}

func newFlow00(t *testing.T, pool *pgxpool.Pool) flow00 {
	t.Helper()
	clk := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	pinned := &replay.Clock{}
	consumers := system.New(module.Deps{Clock: pinned, Pool: pool}).Consumers()
	return flow00{
		pool: pool, clock: clk, pinned: pinned, echo: replay.Handlers(consumers)[0],
		uow: db.New(pool, testkit.NewIDs(1), clk), echoUoW: db.New(pool, testkit.NewIDs(1), pinned),
	}
}

func (f *flow00) ping(t *testing.T, raw string) {
	t.Helper()
	g := testkit.NewIDs(uint64(len(f.events)) + 10)
	user, err := ids.ParseUserID(g.NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	note, err := domain.ParseNote(raw)
	if err != nil {
		t.Fatal(err)
	}
	ctx := observability.WithActor(t.Context(), "user:"+user.String())
	cmd := app.RecordPing{ID: g.NewV7(), UserID: user, Note: note}
	if _, err := app.NewRecordPingHandler(f.uow).Handle(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	var id uuid.UUID
	if err := f.pool.QueryRow(t.Context(), `SELECT id FROM events ORDER BY id DESC LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	f.events = append(f.events, id)
	f.clock.Advance(time.Second)
}

func (f *flow00) echoAll(t *testing.T) replay.Report {
	t.Helper()
	rep, err := replay.Backfill(t.Context(), replay.BackfillOptions{
		Pool:    f.pool,
		UoW:     f.echoUoW,
		Clock:   f.pinned,
		Now:     f.clock,
		Handler: f.echo,
		Types:   []events.Type{events.TypeSystemPinged},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Minute)
	return rep
}

func echoedFlow00(t *testing.T) flow00 {
	t.Helper()
	f := newFlow00(t, testkit.DB(t))
	f.ping(t, "hi")
	f.echoAll(t)
	f.ping(t, "there")
	f.echoAll(t)
	return f
}

func replayInto(ctx context.Context, source, target *pgxpool.Pool, o replay.Options) (replay.Report, error) {
	clk := &replay.Clock{}
	uow := db.New(target, testkit.NewIDs(2), clk)
	if o.Handlers == nil {
		o.Handlers = replay.Handlers(system.New(module.Deps{Clock: clk, Pool: target, UoW: uow}).Consumers())
	}
	o.Source, o.Target, o.UoW, o.Clock = source, target, uow, clk
	return replay.Run(ctx, o)
}

func echoedAt(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT note || ' ' || echoed_at::text FROM system_pings ORDER BY note`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func TestRun_verifyReportsZeroDiffsOnFlow00State(t *testing.T) {
	t.Parallel()
	f := echoedFlow00(t)
	t.Run("target", func(t *testing.T) {
		t.Parallel()
		target := testkit.DB(t)
		rep, err := replayInto(t.Context(), f.pool, target, replay.Options{Verify: true})
		if err != nil {
			t.Fatal(err)
		}
		if rep.Events != 2 || rep.Applied != 2 || rep.Duplicates != 0 || len(rep.Diffs) != 0 {
			t.Fatalf("report = %+v, want 2 events, 2 applied, no diffs", rep)
		}
		want, got := echoedAt(t, f.pool), echoedAt(t, target)
		if strings.Join(got, "|") != strings.Join(want, "|") || len(got) != 2 {
			t.Fatalf("replayed echoed_at = %q, want %q", got, want)
		}
		t.Logf("replay --verify on flow 00 state: %d events, %d applied, %d diffs; echoed_at %q",
			rep.Events, rep.Applied, len(rep.Diffs), got)
	})
}

type tickingClock struct{ *testkit.Clock }

func (c tickingClock) Now() time.Time {
	c.Advance(time.Microsecond)
	return c.Clock.Now()
}

func TestRun_verifyReportsZeroDiffsOnLiveDispatchedState(t *testing.T) {
	t.Parallel()
	f := newFlow00(t, testkit.DB(t))
	f.ping(t, "hi")
	f.ping(t, "there")
	tick := tickingClock{testkit.NewClock(time.Date(2026, 3, 1, 13, 0, 0, 0, time.UTC))}
	uow := db.New(f.pool, testkit.NewIDs(3), tick)
	echo := replay.Handlers(system.New(module.Deps{Clock: tick, Pool: f.pool, UoW: uow}).Consumers())[0]
	for _, id := range f.events {
		var payload []byte
		row := f.pool.QueryRow(t.Context(), `SELECT payload FROM events WHERE id = $1`, id)
		if err := row.Scan(&payload); err != nil {
			t.Fatal(err)
		}
		ev, err := events.Decode(events.TypeSystemPinged, 1, payload)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := bus.Deliver(t.Context(), uow, tick, echo, ids.EventIDFrom(id), ev); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("target", func(t *testing.T) {
		t.Parallel()
		target := testkit.DB(t)
		rep, err := replayInto(t.Context(), f.pool, target, replay.Options{Verify: true})
		if err != nil {
			t.Fatal(err)
		}
		if rep.Events != 2 || rep.Applied != 2 || len(rep.Diffs) != 0 {
			t.Fatalf("report = %+v, want 2 events, 2 applied, no diffs", rep)
		}
		want, got := echoedAt(t, f.pool), echoedAt(t, target)
		if strings.Join(got, "|") != strings.Join(want, "|") || len(got) != 2 {
			t.Fatalf("replayed echoed_at = %q, want %q", got, want)
		}
	})
}

func TestRun_stopsAtToAndVerifyNamesTheRowsReplayDidNotBuild(t *testing.T) {
	t.Parallel()
	f := echoedFlow00(t)
	t.Run("target", func(t *testing.T) {
		t.Parallel()
		target := testkit.DB(t)
		rep, err := replayInto(t.Context(), f.pool, target, replay.Options{To: f.events[0], Verify: true})
		if err != nil {
			t.Fatal(err)
		}
		if rep.Events != 1 || len(rep.Diffs) != 1 ||
			!strings.HasPrefix(rep.Diffs[0], `system_pings: only in source: {"id": "`) ||
			!strings.Contains(rep.Diffs[0], `"note": "there"`) {
			t.Fatalf("report = %+v, want one event and one source-only system_pings row", rep)
		}
	})
}

func TestRun_refusesATargetThatAlreadyHoldsEvents(t *testing.T) {
	t.Parallel()
	f := echoedFlow00(t)
	_, err := replayInto(t.Context(), f.pool, f.pool, replay.Options{})
	var refused replay.RefusedError
	if !errorsAs(err, &refused) || errs.CodeOf(err) != errs.CodeInvalidInput ||
		string(refused) != "target holds 2 events; replay writes only into a fresh database" {
		t.Fatalf("replay into the source = %v, want a refusal", err)
	}
}

func TestRun_neverRebuildsLedgersAndReportsLedgerCheckDiffs(t *testing.T) {
	t.Parallel()
	f := echoedFlow00(t)
	var checked *pgxpool.Pool
	check := replay.LedgerCheck{
		Name: "pings", Tables: []string{"system_pings"}, Handlers: []string{f.echo.Name},
		Check: func(_ context.Context, source *pgxpool.Pool) ([]string, error) {
			checked = source
			return []string{"balance off by 1"}, nil
		},
	}
	t.Run("target", func(t *testing.T) {
		t.Parallel()
		target := testkit.DB(t)
		rep, err := replayInto(t.Context(), f.pool, target, replay.Options{
			Verify: true, Checks: []replay.LedgerCheck{check},
		})
		if err != nil {
			t.Fatal(err)
		}
		if rep.Applied != 0 || len(rep.Diffs) != 1 || rep.Diffs[0] != "ledger pings: balance off by 1" ||
			checked != f.pool {
			t.Fatalf("report = %+v, want the ledger handler skipped and one ledger diff from the source", rep)
		}
		if got := echoedAt(t, target); len(got) != 0 {
			t.Fatalf("replay rebuilt the ledger table: %q", got)
		}
	})
}

func TestRun_failsWhenALedgerCheckFails(t *testing.T) {
	t.Parallel()
	f := echoedFlow00(t)
	check := replay.LedgerCheck{Name: "broken", Check: func(context.Context, *pgxpool.Pool) ([]string, error) {
		return nil, errs.New(errs.CodeDBUnavailable, "fixture.check")
	}}
	t.Run("target", func(t *testing.T) {
		t.Parallel()
		target := testkit.DB(t)
		_, err := replayInto(t.Context(), f.pool, target, replay.Options{
			Verify: true, Checks: []replay.LedgerCheck{check},
		})
		if errs.CodeOf(err) != errs.CodeDBUnavailable {
			t.Fatalf("err = %v, want the check's db_unavailable", err)
		}
	})
}

func TestRun_failsOnAHandlerErrorAndOnAnUndecodablePayload(t *testing.T) {
	t.Parallel()
	f := echoedFlow00(t)
	boom := bus.Handle("boom", func(context.Context, db.Tx, events.SystemPinged, time.Time) error {
		return errs.New(errs.CodeInternal, "fixture.boom")
	})
	t.Run("target", func(t *testing.T) {
		t.Parallel()
		target := testkit.DB(t)
		_, err := replayInto(t.Context(), f.pool, target, replay.Options{Handlers: []bus.HandlerSpec{boom}})
		if err == nil || !strings.HasPrefix(err.Error(), "replay.deliver: internal: fixture.boom") {
			t.Fatalf("err = %v, want the handler's error", err)
		}
	})
	t.Run("undecodable", func(t *testing.T) {
		t.Parallel()
		g := newFlow00(t, testkit.DB(t))
		insertRaw(t, g.pool, `{"v": 9}`)
		t.Run("target", func(t *testing.T) {
			t.Parallel()
			target := testkit.DB(t)
			if _, err := replayInto(
				t.Context(),
				g.pool,
				target,
				replay.Options{},
			); errs.CodeOf(
				err,
			) != errs.CodeDecodeFailed {
				t.Fatalf("err = %v, want decode_failed", err)
			}
		})
	})
}

func insertRaw(t *testing.T, pool *pgxpool.Pool, payload string) {
	t.Helper()
	if _, err := pool.Exec(
		t.Context(),
		`INSERT INTO events (id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id)
		VALUES ($1, 'system', $1, 'system.pinged', $2, 'system', 'test')`,
		testkit.NewIDs(99).NewV7(),
		payload,
	); err != nil {
		t.Fatal(err)
	}
}

func TestRun_failsWhenATargetOrSourceCannotBeRead(t *testing.T) {
	t.Parallel()
	f := echoedFlow00(t)
	t.Run("closed target", func(t *testing.T) {
		t.Parallel()
		target := testkit.DB(t)
		target.Close()
		if _, err := replayInto(t.Context(), f.pool, target, replay.Options{}); errs.CodeOf(err) != errs.CodeInternal {
			t.Fatalf("err = %v, want internal", err)
		}
	})
	t.Run("closed source", func(t *testing.T) {
		t.Parallel()
		source := testkit.DB(t)
		source.Close()
		t.Run("target", func(t *testing.T) {
			t.Parallel()
			if _, err := replayInto(t.Context(), source, testkit.DB(t), replay.Options{}); err == nil ||
				!strings.HasPrefix(err.Error(), "replay.load: internal") {
				t.Fatalf("err = %v, want load to fail", err)
			}
		})
	})
	t.Run("target refuses the event", func(t *testing.T) {
		t.Parallel()
		target := testkit.DB(t)
		exec(t, target, `ALTER TABLE events ADD CONSTRAINT refuse CHECK (false) NOT VALID`)
		if _, err := replayInto(t.Context(), f.pool, target, replay.Options{}); err == nil ||
			!strings.HasPrefix(err.Error(), "replay.Run: internal") {
			t.Fatalf("err = %v, want the insert to fail", err)
		}
	})
}

func TestRun_failsWhenTheSourceSchemaIsMissingATable(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		drop, want string
		verify     bool
	}{
		"deliveries": {drop: "event_deliveries", want: "replay.handledAt: internal"},
		"projection": {drop: "system_pings", want: "replay.sortedRows: internal", verify: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			source := testkit.DB(t)
			exec(t, source, `DROP TABLE `+tc.drop)
			t.Run("target", func(t *testing.T) {
				t.Parallel()
				_, err := replayInto(t.Context(), source, testkit.DB(t), replay.Options{Verify: tc.verify})
				if err == nil || !strings.HasPrefix(err.Error(), tc.want) {
					t.Fatalf("err = %v, want %s", err, tc.want)
				}
			})
		})
	}
}

func exec(t *testing.T, pool *pgxpool.Pool, sql string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), sql); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterLedgerCheck_addsTheCheckReplayRunsAndRejectsARepeatedName(t *testing.T) {
	t.Parallel()
	replay.EmptyLedgerChecks(t)
	replay.RegisterLedgerCheck(replay.LedgerCheck{Name: "fixture"})
	func() {
		defer func() {
			if recover() == nil {
				t.Error("second fixture registered, want a panic")
			}
		}()
		replay.RegisterLedgerCheck(replay.LedgerCheck{Name: "fixture"})
	}()
	got := replay.LedgerChecks()
	if len(got) != 1 || got[0].Name != "fixture" {
		t.Fatalf("LedgerChecks() = %+v, want the fixture once", got)
	}
}
