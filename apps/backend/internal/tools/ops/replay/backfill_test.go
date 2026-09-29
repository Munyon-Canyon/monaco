package replay_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/tools/ops/replay"
)

func errorsAs(err error, target any) bool { return errors.As(err, target) }

func TestBackfill_deliversEachEventOnceThroughTheDispatchWrapper(t *testing.T) {
	t.Parallel()
	f := newFlow00(t, testkit.DB(t))
	f.ping(t, "hi")
	f.ping(t, "there")
	if rep := f.echoAll(t); rep.Events != 2 || rep.Applied != 2 || rep.Duplicates != 0 {
		t.Fatalf("first backfill = %+v, want 2 applied", rep)
	}
	if rep := f.echoAll(t); rep.Events != 2 || rep.Applied != 0 || rep.Duplicates != 2 {
		t.Fatalf("second backfill = %+v, want 2 duplicates", rep)
	}
	if got := echoedAt(t, f.pool); len(got) != 2 || got[0] != "hi 2026-03-01 12:00:02+00" {
		t.Fatalf("echoed = %q, want both pings echoed at the backfill's clock", got)
	}
}

func TestBackfill_startsAfterSince(t *testing.T) {
	t.Parallel()
	f := newFlow00(t, testkit.DB(t))
	f.ping(t, "hi")
	f.ping(t, "there")
	rep, err := replay.Backfill(t.Context(), replay.BackfillOptions{
		Pool: f.pool, UoW: f.echoUoW, Clock: f.pinned, Now: f.clock, Handler: f.echo, Since: f.events[0],
	})
	if err != nil || rep.Events != 1 || rep.Applied != 1 {
		t.Fatalf("backfill since the first ping = %+v, %v, want only the second", rep, err)
	}
}

func TestBackfill_refusesTypesTheHandlerDoesNotHandle(t *testing.T) {
	t.Parallel()
	f := newFlow00(t, testkit.DB(t))
	_, err := replay.Backfill(t.Context(), replay.BackfillOptions{
		Pool:    f.pool,
		UoW:     f.echoUoW,
		Clock:   f.pinned,
		Now:     f.clock,
		Handler: f.echo,
		Types:   []events.Type{"cabal.created"},
	})
	var refused replay.RefusedError
	if !errorsAs(err, &refused) ||
		!strings.HasSuffix(err.Error(), ": handler system.echo handles system.pinged, not cabal.created") {
		t.Fatalf("err = %v, want a refusal", err)
	}
}

func TestBackfill_failsOnAnUnreadableLogOrAnUndecodableEvent(t *testing.T) {
	t.Parallel()
	f := newFlow00(t, testkit.DB(t))
	insertRaw(t, f.pool, `{"v": 9}`)
	if _, err := replay.Backfill(t.Context(), replay.BackfillOptions{
		Pool: f.pool, UoW: f.echoUoW, Clock: f.pinned, Now: f.clock, Handler: f.echo,
	}); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("err = %v, want decode_failed", err)
	}
	f.pool.Close()
	if _, err := replay.Backfill(t.Context(), replay.BackfillOptions{
		Pool: f.pool, UoW: f.echoUoW, Clock: f.pinned, Now: f.clock, Handler: f.echo,
	}); err == nil || !strings.HasPrefix(err.Error(), "replay.load: internal") {
		t.Fatalf("err = %v, want load to fail", err)
	}
}
