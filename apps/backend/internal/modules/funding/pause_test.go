package funding_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type pauseHints struct {
	mu   sync.Mutex
	keys []string
}

func (h *pauseHints) PublishHint(_ context.Context, key string, _ []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.keys = append(h.keys, key)
}

func (h *pauseHints) all() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.keys)
}

type pauseEnv struct {
	pool   *pgxpool.Pool
	uow    *db.UnitOfWork
	clock  clock.Clock
	hints  *pauseHints
	pause  *app.PauseCabalHandler
	resume *app.ResumeCabalHandler
}

func newPauseEnv(t *testing.T) pauseEnv {
	t.Helper()
	pool := testkit.DB(t)
	clk := testkit.NewClock(clock.Real{}.Now().UTC())
	uow := db.New(pool, testkit.NewIDs(20), clk)
	h := &pauseHints{}
	return pauseEnv{
		pool: pool, uow: uow, clock: clk, hints: h,
		pause:  app.NewPauseCabalHandler(uow, ids.Real{}, clk, h),
		resume: app.NewResumeCabalHandler(uow, clk, h),
	}
}

func opsContext(t *testing.T) context.Context {
	t.Helper()
	return observability.WithActor(t.Context(), "system:monacoctl")
}

func countEvents(t *testing.T, pool *pgxpool.Pool, typ events.Type) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE type = $1`, typ).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func openPauses(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT reason FROM cabal_pauses WHERE resolved_at IS NULL ORDER BY created_at`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var reasons []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			t.Fatal(err)
		}
		reasons = append(reasons, r)
	}
	return reasons
}

func eventPayload(t *testing.T, pool *pgxpool.Pool, typ events.Type, into any) {
	t.Helper()
	var raw []byte
	if err := pool.QueryRow(t.Context(), `SELECT payload FROM events WHERE type = $1`, typ).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatal(err)
	}
}

func pausedEvent(t *testing.T, pool *pgxpool.Pool) events.CabalPaused {
	t.Helper()
	var e events.CabalPaused
	eventPayload(t, pool, events.TypeCabalPaused, &e)
	return e
}

func resumedEvent(t *testing.T, pool *pgxpool.Pool) events.CabalResumed {
	t.Helper()
	var e events.CabalResumed
	eventPayload(t, pool, events.TypeCabalResumed, &e)
	return e
}

func sameCabal(got *uuid.UUID, want ids.CabalID) bool { return got != nil && *got == want.UUID() }

func TestPause_OpsThenResume(t *testing.T) {
	t.Parallel()
	env := newPauseEnv(t)
	cabal := testkit.NewCabal(t, env.pool)
	actor := cabal.Creator.ID
	pauseID, err := env.pause.Handle(opsContext(t), app.PauseCabal{
		CabalID: &cabal.ID, Reason: domain.PauseReasonOps, Note: "incident 7", Actor: &actor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.resume.Handle(opsContext(t), app.ResumeCabal{CabalID: &cabal.ID, Actor: &actor}); err != nil {
		t.Fatal(err)
	}
	assertOneCycle(t, env.pool, pauseID, cabal.ID)
	var note string
	var createdBy, resolvedBy uuid.UUID
	if err := env.pool.QueryRow(t.Context(),
		`SELECT note, created_by, resolved_by FROM cabal_pauses WHERE id = $1 AND resolved_at IS NOT NULL`, pauseID,
	).Scan(&note, &createdBy, &resolvedBy); err != nil {
		t.Fatal(err)
	}
	if note != "incident 7" || createdBy != actor.UUID() || resolvedBy != actor.UUID() {
		t.Fatalf("row note=%q created_by=%s resolved_by=%s", note, createdBy, resolvedBy)
	}
	want := "cabal." + cabal.ID.String() + ".pause_changed"
	if got := env.hints.all(); !slices.Equal(got, []string{want, want}) {
		t.Fatalf("hints = %v, want two %s", got, want)
	}
}

func assertOneCycle(t *testing.T, pool *pgxpool.Pool, pauseID uuid.UUID, cabal ids.CabalID) {
	t.Helper()
	p, r := countEvents(t, pool, events.TypeCabalPaused), countEvents(t, pool, events.TypeCabalResumed)
	if p != 1 || r != 1 {
		t.Fatalf("cabal.paused=%d cabal.resumed=%d, want 1 1", p, r)
	}
	paused := pausedEvent(t, pool)
	if paused.PauseID != pauseID || !sameCabal(paused.CabalID, cabal) || paused.Reason != "ops" ||
		paused.Scope != "cabal" {
		t.Fatalf("cabal.paused = %+v", paused)
	}
	if resumed := resumedEvent(t, pool); !sameCabal(resumed.CabalID, cabal) || resumed.Scope != "cabal" {
		t.Fatalf("cabal.resumed = %+v", resumed)
	}
}

func TestPause_SecondReasonNoEvent(t *testing.T) {
	t.Parallel()
	env := newPauseEnv(t)
	cabal := testkit.NewCabal(t, env.pool)
	ctx := opsContext(t)
	external, err := env.pause.Handle(
		ctx,
		app.PauseCabal{CabalID: &cabal.ID, Reason: domain.PauseReasonExternalDeposit},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.pause.Handle(ctx, app.PauseCabal{CabalID: &cabal.ID, Reason: domain.PauseReasonOps}); err != nil {
		t.Fatal(err)
	}
	if n := countEvents(t, env.pool, events.TypeCabalPaused); n != 1 {
		t.Fatalf("cabal.paused = %d after two reasons, want 1", n)
	}
	err = env.resume.Handle(ctx, app.ResumeCabal{CabalID: &cabal.ID})
	if errs.CodeOf(err) != errs.CodeCabalStillPaused {
		t.Fatalf("resume error = %v, want %s", err, errs.CodeCabalStillPaused)
	}
	if !slices.ContainsFunc(errs.Detail(err), func(a slog.Attr) bool {
		return a.Key == "reasons" && a.Value.String() == "external_deposit"
	}) {
		t.Fatalf("detail = %v, want reasons=external_deposit", errs.Detail(err))
	}
	if got := openPauses(t, env.pool); !slices.Equal(got, []string{"external_deposit"}) {
		t.Fatalf("open pauses = %v, want the ops row resolved and committed", got)
	}
	if n := countEvents(t, env.pool, events.TypeCabalResumed); n != 0 {
		t.Fatalf("cabal.resumed = %d while external_deposit is open, want 0", n)
	}
	err = env.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return app.ResolvePause(ctx, tx, env.clock.Now(), env.hints, external)
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := countEvents(t, env.pool, events.TypeCabalResumed); n != 1 {
		t.Fatalf("cabal.resumed = %d after the last reason resolved, want 1", n)
	}
}

func TestPause_GlobalPausesEveryCabal(t *testing.T) {
	t.Parallel()
	own, other := ids.CabalIDFrom(testkit.NewIDs(30).NewV7()), ids.CabalIDFrom(testkit.NewIDs(31).NewV7())
	open := []domain.Pause{
		{CabalID: &other, Reason: domain.PauseReasonExternalDeposit},
		{Reason: domain.PauseReasonOps},
	}
	paused, reasons := domain.IsPaused(open, own)
	if !paused || !slices.Equal(reasons, []domain.PauseReason{domain.PauseReasonOps}) {
		t.Fatalf("IsPaused(no own row, global ops) = %v %v, want true [ops]", paused, reasons)
	}

	env := newPauseEnv(t)
	ctx := opsContext(t)
	if _, err := env.pause.Handle(ctx, app.PauseCabal{Reason: domain.PauseReasonOps, Note: "all"}); err != nil {
		t.Fatal(err)
	}
	if err := env.resume.Handle(ctx, app.ResumeCabal{}); err != nil {
		t.Fatal(err)
	}
	pausedEvent := pausedEvent(t, env.pool)
	resumedEvent := resumedEvent(t, env.pool)
	if pausedEvent.CabalID != nil || pausedEvent.Scope != "global" || resumedEvent.CabalID != nil ||
		resumedEvent.Scope != "global" {
		t.Fatalf("events = %+v %+v, want global scope with no cabal_id", pausedEvent, resumedEvent)
	}
	want := "cabal.global.pause_changed"
	if got := env.hints.all(); !slices.Equal(got, []string{want, want}) {
		t.Fatalf("hints = %v, want two %s", got, want)
	}
}

func TestPause_IsPausedListsGlobalReasonsFirstOnceEach(t *testing.T) {
	t.Parallel()
	own, other := ids.CabalIDFrom(testkit.NewIDs(30).NewV7()), ids.CabalIDFrom(testkit.NewIDs(31).NewV7())
	open := []domain.Pause{
		{CabalID: &own, Reason: domain.PauseReasonExternalDeposit},
		{CabalID: &own, Reason: domain.PauseReasonOps},
		{CabalID: &own, Reason: domain.PauseReasonExternalDeposit},
		{Reason: domain.PauseReasonOps},
	}
	paused, reasons := domain.IsPaused(open, own)
	want := []domain.PauseReason{domain.PauseReasonOps, domain.PauseReasonExternalDeposit}
	if !paused || !slices.Equal(reasons, want) {
		t.Fatalf("IsPaused = %v %v, want true %v", paused, reasons, want)
	}
	if paused, reasons := domain.IsPaused(open[:3], other); paused || reasons != nil {
		t.Fatalf("IsPaused(other cabal) = %v %v, want false nil", paused, reasons)
	}
}

func TestPause_ConcurrentPausesAppendOnePausedEvent(t *testing.T) {
	t.Parallel()
	env := newPauseEnv(t)
	cabal := testkit.NewCabal(t, env.pool)
	ctx := opsContext(t)
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]error, 8)
	for i := range results {
		wg.Go(func() {
			<-start
			_, results[i] = env.pause.Handle(ctx, app.PauseCabal{CabalID: &cabal.ID, Reason: domain.PauseReasonOps})
		})
	}
	close(start)
	wg.Wait()
	for _, err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	if n, open := countEvents(
		t,
		env.pool,
		events.TypeCabalPaused,
	), openPauses(
		t,
		env.pool,
	); n != 1 ||
		len(open) != len(results) {
		t.Fatalf("cabal.paused=%d open=%v, want 1 event over %d rows", n, open, len(results))
	}
}

func TestPause_ResumeWithNothingOpenAppendsNothing(t *testing.T) {
	t.Parallel()
	env := newPauseEnv(t)
	cabal := testkit.NewCabal(t, env.pool)
	if err := env.resume.Handle(opsContext(t), app.ResumeCabal{CabalID: &cabal.ID}); err != nil {
		t.Fatal(err)
	}
	if n := countEvents(t, env.pool, events.TypeCabalResumed); n != 0 || len(env.hints.all()) != 0 {
		t.Fatalf("cabal.resumed=%d hints=%v, want none", n, env.hints.all())
	}
}

func TestResolvePause_ConvergesAndRefusesUnknownIDs(t *testing.T) {
	t.Parallel()
	env := newPauseEnv(t)
	cabal := testkit.NewCabal(t, env.pool)
	ctx := opsContext(t)
	id, err := env.pause.Handle(ctx, app.PauseCabal{CabalID: &cabal.ID, Reason: domain.PauseReasonExternalDeposit})
	if err != nil {
		t.Fatal(err)
	}
	resolve := func(id uuid.UUID) error {
		return env.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
			return app.ResolvePause(ctx, tx, env.clock.Now(), env.hints, id)
		})
	}
	for range 2 {
		if err := resolve(id); err != nil {
			t.Fatal(err)
		}
	}
	if n := countEvents(t, env.pool, events.TypeCabalResumed); n != 1 {
		t.Fatalf("cabal.resumed = %d after resolving twice, want 1", n)
	}
	if err := resolve(testkit.NewIDs(32).NewV7()); err == nil {
		t.Fatal("ResolvePause(unknown id) error = nil")
	}
}

func TestResolvePause_ResumesTheGlobalScope(t *testing.T) {
	t.Parallel()
	env := newPauseEnv(t)
	ctx := opsContext(t)
	id, err := env.pause.Handle(ctx, app.PauseCabal{Reason: domain.PauseReasonExternalDeposit})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return app.ResolvePause(ctx, tx, env.clock.Now(), env.hints, id)
	}); err != nil {
		t.Fatal(err)
	}
	resumed := resumedEvent(t, env.pool)
	if resumed.CabalID != nil || resumed.Scope != "global" {
		t.Fatalf("cabal.resumed = %+v, want the global scope", resumed)
	}
}

func TestPause_FailuresRollBack(t *testing.T) {
	t.Parallel()
	ops := domain.PauseReasonOps
	cases := map[string]func(t *testing.T, env pauseEnv, cabal ids.CabalID, mark func()) error{
		"unknown cabal": func(t *testing.T, env pauseEnv, _ ids.CabalID, _ func()) error {
			t.Helper()
			missing := ids.CabalIDFrom(testkit.NewIDs(33).NewV7())
			_, err := env.pause.Handle(opsContext(t), app.PauseCabal{CabalID: &missing, Reason: ops})
			return err
		},
		"pause without an actor": func(t *testing.T, env pauseEnv, cabal ids.CabalID, _ func()) error {
			t.Helper()
			_, err := env.pause.Handle(t.Context(), app.PauseCabal{CabalID: &cabal, Reason: ops})
			return err
		},
		"resume without an actor": func(t *testing.T, env pauseEnv, cabal ids.CabalID, mark func()) error {
			t.Helper()
			mustPause(t, env, cabal, ops)
			mark()
			return env.resume.Handle(t.Context(), app.ResumeCabal{CabalID: &cabal})
		},
		"resolve without an actor": func(t *testing.T, env pauseEnv, cabal ids.CabalID, mark func()) error {
			t.Helper()
			id := mustPause(t, env, cabal, domain.PauseReasonExternalDeposit)
			mark()
			return env.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
				return app.ResolvePause(ctx, tx, env.clock.Now(), env.hints, id)
			})
		},
		"pause on a missing table": func(t *testing.T, env pauseEnv, cabal ids.CabalID, _ func()) error {
			t.Helper()
			exec(t, env.pool, `ALTER TABLE cabal_pauses RENAME TO cabal_pauses_gone`)
			_, err := env.pause.Handle(opsContext(t), app.PauseCabal{CabalID: &cabal, Reason: ops})
			return err
		},
		"resume on a missing table": func(t *testing.T, env pauseEnv, cabal ids.CabalID, _ func()) error {
			t.Helper()
			exec(t, env.pool, `ALTER TABLE cabal_pauses RENAME TO cabal_pauses_gone`)
			return env.resume.Handle(opsContext(t), app.ResumeCabal{CabalID: &cabal})
		},
		"resume when the update fails": func(t *testing.T, env pauseEnv, cabal ids.CabalID, mark func()) error {
			t.Helper()
			mustPause(t, env, cabal, ops)
			mark()
			failUpdates(t, env.pool)
			return env.resume.Handle(opsContext(t), app.ResumeCabal{CabalID: &cabal})
		},
		"resolve when the update fails": func(t *testing.T, env pauseEnv, cabal ids.CabalID, mark func()) error {
			t.Helper()
			id := mustPause(t, env, cabal, ops)
			mark()
			failUpdates(t, env.pool)
			return env.uow.Do(opsContext(t), func(ctx context.Context, tx db.Tx) error {
				return app.ResolvePause(ctx, tx, env.clock.Now(), env.hints, id)
			})
		},
		"pause while the scope lock is held": func(t *testing.T, env pauseEnv, cabal ids.CabalID, _ func()) error {
			t.Helper()
			holdScopeLock(t, env.pool, cabal.String())
			ctx, cancel := context.WithTimeout(opsContext(t), 300*time.Millisecond)
			defer cancel()
			_, err := env.pause.Handle(ctx, app.PauseCabal{CabalID: &cabal, Reason: ops})
			return err
		},
		"resolve while the scope lock is held": func(t *testing.T, env pauseEnv, cabal ids.CabalID, mark func()) error {
			t.Helper()
			id := mustPause(t, env, cabal, ops)
			mark()
			holdScopeLock(t, env.pool, cabal.String())
			err := env.uow.Do(opsContext(t), func(ctx context.Context, tx db.Tx) error {
				if _, err := tx.Queries().Exec(ctx, `SET LOCAL lock_timeout = '10ms'`); err != nil {
					t.Fatal(err)
				}
				return app.ResolvePause(ctx, tx, env.clock.Now(), env.hints, id)
			})
			var pg *pgconn.PgError
			if !errors.As(err, &pg) || pg.Code != "55P03" {
				t.Fatalf("ResolvePause error = %v, want lock_not_available from the scope lock", err)
			}
			return err
		},
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			env := newPauseEnv(t)
			cabal := testkit.NewCabal(t, env.pool)
			var before, hintsBefore int
			mark := func() {
				before = countEvents(
					t,
					env.pool,
					events.TypeCabalPaused,
				) + countEvents(
					t,
					env.pool,
					events.TypeCabalResumed,
				)
				hintsBefore = len(env.hints.all())
			}
			mark()
			if err := run(t, env, cabal.ID, mark); err == nil {
				t.Fatal("error = nil")
			}
			after := countEvents(
				t,
				env.pool,
				events.TypeCabalPaused,
			) + countEvents(
				t,
				env.pool,
				events.TypeCabalResumed,
			)
			if after != before || len(env.hints.all()) != hintsBefore {
				t.Fatalf("events %d -> %d, hints %d -> %d; a failed command must append and publish nothing",
					before, after, hintsBefore, len(env.hints.all()))
			}
		})
	}
}

func mustPause(t *testing.T, env pauseEnv, cabal ids.CabalID, reason domain.PauseReason) uuid.UUID {
	t.Helper()
	id, err := env.pause.Handle(opsContext(t), app.PauseCabal{CabalID: &cabal, Reason: reason})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func exec(t *testing.T, pool *pgxpool.Pool, sql string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), sql); err != nil {
		t.Fatal(err)
	}
}

func failUpdates(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	exec(t, pool, `CREATE FUNCTION refuse_pause_update() RETURNS trigger LANGUAGE plpgsql
		AS $$ BEGIN RAISE EXCEPTION 'refused'; END $$`)
	exec(t, pool, `CREATE TRIGGER refuse_pause_update BEFORE UPDATE ON cabal_pauses
		FOR EACH ROW EXECUTE FUNCTION refuse_pause_update()`)
}

func holdScopeLock(t *testing.T, pool *pgxpool.Pool, scope string) {
	t.Helper()
	conn, err := pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	key := `hashtextextended('cabal-pause:' || $1::text, 0)`
	if _, err := conn.Exec(t.Context(), `SELECT pg_advisory_lock(`+key+`)`, scope); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock(`+key+`)`, scope)
		conn.Release()
	})
}
