package system_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/system/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/system/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type flagFixture struct {
	fixture
	clock   *testkit.Clock
	handler *app.FlagPingHandler
	admin   ids.UserID
}

func newFlagFixture(t *testing.T) flagFixture {
	t.Helper()
	f := newFixture(t)
	clk := testkit.NewClock(f.now)
	return flagFixture{
		fixture: f, clock: clk, handler: app.NewFlagPingHandler(f.uow, f.ids, clk), admin: userID(t, f.ids),
	}
}

func (f flagFixture) flag(t *testing.T, ping uuid.UUID) error {
	t.Helper()
	reason, err := events.NewReason("spam")
	if err != nil {
		t.Fatal(err)
	}
	ctx := observability.WithActor(t.Context(), "admin:"+f.admin.String())
	return f.handler.Handle(ctx, app.FlagPing{PingID: ping, AdminID: f.admin, Reason: reason})
}

func (f flagFixture) flaggedAt(t *testing.T, ping uuid.UUID) *time.Time {
	t.Helper()
	var at *time.Time
	err := f.pool.QueryRow(t.Context(), `SELECT flagged_at FROM system_pings WHERE id = $1`, ping).Scan(&at)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

type flagEvent struct {
	Type    string
	Actor   string
	Payload []byte
}

func (f flagFixture) flagEvents(t *testing.T) []flagEvent {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT type, actor_type || ':' || actor_id, payload FROM events
		WHERE type IN ('system.ping_flagged', 'admin.action') ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowToStructByPos[flagEvent])
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestFlagPing_flagsThePingAndAppendsBothEventsAsTheAdminInOneTransaction(t *testing.T) {
	t.Parallel()
	f := newFlagFixture(t)
	ping, err := f.record(t, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.flag(t, ping.ID); err != nil {
		t.Fatal(err)
	}
	if at := f.flaggedAt(t, ping.ID); at == nil || !at.Equal(f.now) {
		t.Fatalf("flagged_at = %v, want %s", at, f.now)
	}
	got := f.flagEvents(t)
	by := "admin:" + f.admin.String()
	if len(got) != 2 || got[0].Type != "system.ping_flagged" || got[1].Type != "admin.action" ||
		got[0].Actor != by || got[1].Actor != by {
		t.Fatalf("events = %+v, want system.ping_flagged then admin.action, both by %s", got, by)
	}
	f.expectPingFlagged(t, got[0].Payload, ping.ID)
	f.expectAdminAction(t, got[1].Payload, ping.ID)
}

func (f flagFixture) expectPingFlagged(t *testing.T, payload []byte, ping uuid.UUID) {
	t.Helper()
	var got events.SystemPingFlagged
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	want := events.SystemPingFlagged{V: 1, PingID: ping, AdminID: f.admin.UUID(), FlaggedAt: f.now}
	if got != want {
		t.Fatalf("system.ping_flagged = %+v, want %+v", got, want)
	}
}

func (f flagFixture) expectAdminAction(t *testing.T, payload []byte, ping uuid.UUID) {
	t.Helper()
	var got events.AdminAction
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	before, after := got.Before, got.After
	got.Before, got.After = nil, nil
	want := events.AdminAction{
		V: 1, ActionID: got.ActionID, AdminID: f.admin.UUID(), Action: events.AdminActionPingFlag,
		TargetType: events.AdminTargetSystemPing, TargetID: ping.String(), Reason: "spam",
	}
	if !reflect.DeepEqual(got, want) || got.ActionID == uuid.Nil {
		t.Fatalf("admin.action = %+v, want %+v with an action id", got, want)
	}
	if !sameJSON(t, before, `{"flagged_at":null}`) || !sameJSON(t, after, `{"flagged_at":"2026-03-01T12:00:00Z"}`) {
		t.Fatalf("admin.action before %s after %s, want the flag time moving from null to %s", before, after, f.now)
	}
}

func sameJSON(t *testing.T, got json.RawMessage, want string) bool {
	t.Helper()
	var a, b any
	if err := json.Unmarshal(got, &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &b); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(a, b)
}

func TestFlagPing_RollsBackTogether(t *testing.T) {
	t.Parallel()
	f := newFlagFixture(t)
	ping, err := f.record(t, "hi")
	if err != nil {
		t.Fatal(err)
	}
	reason, err := events.NewReason("spam")
	if err != nil {
		t.Fatal(err)
	}
	err = f.handler.Handle(t.Context(), app.FlagPing{PingID: ping.ID, AdminID: f.admin, Reason: reason})
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Handle without an actor err = %v, want internal", err)
	}
	if at := f.flaggedAt(t, ping.ID); at != nil {
		t.Fatalf("flagged_at = %v, want null after the rollback", at)
	}
	if got := f.flagEvents(t); len(got) != 0 {
		t.Fatalf("events = %+v, want none after the rollback", got)
	}
}

func TestFlagPing_aZeroReasonFailsAndLeavesThePingUnflagged(t *testing.T) {
	t.Parallel()
	f := newFlagFixture(t)
	ping, err := f.record(t, "hi")
	if err != nil {
		t.Fatal(err)
	}
	ctx := observability.WithActor(t.Context(), "admin:"+f.admin.String())
	err = f.handler.Handle(ctx, app.FlagPing{PingID: ping.ID, AdminID: f.admin})
	if errs.CodeOf(err) != errs.CodeReasonRequired {
		t.Fatalf("Handle with the zero Reason err = %v, want reason_required", err)
	}
	if at, got := f.flaggedAt(t, ping.ID), f.flagEvents(t); at != nil || len(got) != 0 {
		t.Fatalf("flagged_at = %v, events = %+v, want neither after the rollback", at, got)
	}
}

func TestFlagPing_anUnknownPingIsNotFoundAndAppendsNothing(t *testing.T) {
	t.Parallel()
	f := newFlagFixture(t)
	if err := f.flag(t, f.ids.NewV7()); errs.CodeOf(err) != errs.CodeNotFound {
		t.Fatalf("Handle err = %v, want not_found", err)
	}
	if got := f.flagEvents(t); len(got) != 0 {
		t.Fatalf("events = %+v, want none", got)
	}
}

func TestFlagPing_aSecondFlagIsAlreadyFlaggedAndChangesNothing(t *testing.T) {
	t.Parallel()
	f := newFlagFixture(t)
	ping, err := f.record(t, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.flag(t, ping.ID); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Hour)
	if err := f.flag(t, ping.ID); errs.CodeOf(err) != errs.CodeAlreadyFlagged {
		t.Fatalf("second Handle err = %v, want already_flagged", err)
	}
	if at := f.flaggedAt(t, ping.ID); at == nil || !at.Equal(f.now) {
		t.Fatalf("flagged_at = %v, want the first flag %s", at, f.now)
	}
	if got := f.flagEvents(t); len(got) != 2 {
		t.Fatalf("events = %+v, want only the first flag's two events", got)
	}
}

func TestFlagPing_aFlagThatLostTheRaceIsAlreadyFlaggedAndAppendsNothing(t *testing.T) {
	t.Parallel()
	f := newFlagFixture(t)
	ping, err := f.record(t, "hi")
	if err != nil {
		t.Fatal(err)
	}
	for _, ddl := range []string{
		`CREATE FUNCTION skip_update() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NULL; END $$`,
		`CREATE TRIGGER skip_update BEFORE UPDATE ON system_pings FOR EACH ROW EXECUTE FUNCTION skip_update()`,
	} {
		if _, err := f.pool.Exec(t.Context(), ddl); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.flag(t, ping.ID); errs.CodeOf(err) != errs.CodeAlreadyFlagged {
		t.Fatalf("Handle err = %v, want already_flagged when the guarded update changes no row", err)
	}
	if got := f.flagEvents(t); len(got) != 0 {
		t.Fatalf("events = %+v, want none", got)
	}
}

func TestFlagPing_returnsTheReadAndWriteErrors(t *testing.T) {
	t.Parallel()
	for name, ddl := range map[string]string{
		"read":  `ALTER TABLE system_pings RENAME TO system_pings_gone`,
		"write": `ALTER TABLE system_pings ADD CONSTRAINT never_flagged CHECK (flagged_at IS NULL)`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFlagFixture(t)
			ping, err := f.record(t, "hi")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.pool.Exec(t.Context(), ddl); err != nil {
				t.Fatal(err)
			}
			if err := f.flag(t, ping.ID); errs.CodeOf(err) != errs.CodeInternal {
				t.Fatalf("Handle err = %v, want internal", err)
			}
		})
	}
}

func TestFlagPingQuery_changesTheRowOnceAndKeepsTheFirstTime(t *testing.T) {
	t.Parallel()
	f := newFlagFixture(t)
	ping, err := f.record(t, "hi")
	if err != nil {
		t.Fatal(err)
	}
	q := sqlc.New(f.pool)
	first, err := q.FlagPing(t.Context(), sqlc.FlagPingParams{ID: ping.ID, FlaggedAt: f.now})
	if err != nil {
		t.Fatal(err)
	}
	second, err := q.FlagPing(t.Context(), sqlc.FlagPingParams{ID: ping.ID, FlaggedAt: f.now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if at := f.flaggedAt(t, ping.ID); first != 1 || second != 0 || at == nil || !at.Equal(f.now) {
		t.Fatalf(
			"rows changed = %d then %d, flagged_at = %v, want 1 then 0 and the first time %s",
			first,
			second,
			at,
			f.now,
		)
	}
}
