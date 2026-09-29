package system_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/system/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/system/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type fixture struct {
	pool *pgxpool.Pool
	ids  *testkit.IDs
	uow  *db.UnitOfWork
	user ids.UserID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	g := testkit.NewIDs(1)
	pool := testkit.DB(t)
	return fixture{
		pool: pool,
		ids:  g,
		uow:  db.New(pool, g, testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))),
		user: userID(t, g),
	}
}

func userID(t *testing.T, g ids.Generator) ids.UserID {
	t.Helper()
	u, err := ids.ParseUserID(g.NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func note(t *testing.T, raw string) domain.Note {
	t.Helper()
	n, err := domain.ParseNote(raw)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (f fixture) record(t *testing.T, raw string) (app.Ping, error) {
	t.Helper()
	ctx := observability.WithActor(t.Context(), "user:"+f.user.String())
	cmd := app.RecordPing{ID: f.ids.NewV7(), UserID: f.user, Note: note(t, raw)}
	return app.NewRecordPingHandler(f.uow).Handle(ctx, cmd)
}

func (f fixture) eventCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM events`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

type storedEvent struct {
	Type      string
	Aggregate uuid.UUID
	Actor     string
	Event     events.SystemPinged
}

func (f fixture) onlyEvent(t *testing.T) storedEvent {
	t.Helper()
	var (
		e       storedEvent
		payload []byte
	)
	err := f.pool.QueryRow(t.Context(),
		`SELECT type, aggregate_id, actor_type || ':' || actor_id, payload FROM events`).
		Scan(&e.Type, &e.Aggregate, &e.Actor, &payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, &e.Event); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestRecordPing_writesTheRowAndAppendsSystemPingedInOneTransaction(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ping, err := f.record(t, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if ping.Note != "hi" || ping.Echoed || ping.ID == uuid.Nil {
		t.Fatalf("Handle = %+v, want an unechoed ping with note hi", ping)
	}
	want := storedEvent{
		Type: string(events.TypeSystemPinged), Aggregate: ping.ID, Actor: "user:" + f.user.String(),
		Event: events.SystemPinged{V: 1, PingID: ping.ID, UserID: f.user.UUID(), Note: "hi"},
	}
	if got := f.onlyEvent(t); got != want {
		t.Fatalf("events row = %+v, want %+v", got, want)
	}
	got, err := app.GetPing(t.Context(), f.pool, ping.ID, f.user)
	if err != nil || got != ping {
		t.Fatalf("GetPing = %+v, %v, want %+v", got, err, ping)
	}
}

func TestRecordPing_appendsNothingWhenTheRowCannotBeWritten(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE system_pings RENAME TO system_pings_gone`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.record(t, "hi"); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Handle err = %v, want internal", err)
	}
	if n := f.eventCount(t); n != 0 {
		t.Fatalf("events rows = %d, want 0", n)
	}
}

func TestRecordPing_writesNoRowWhenTheEventCannotBeAppended(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cmd := app.RecordPing{ID: f.ids.NewV7(), UserID: f.user, Note: note(t, "hi")}
	if _, err := app.NewRecordPingHandler(f.uow).Handle(t.Context(), cmd); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Handle without an actor err = %v, want internal", err)
	}
	if _, err := app.GetPing(t.Context(), f.pool, cmd.ID, f.user); errs.CodeOf(err) != errs.CodeNotFound {
		t.Fatalf("GetPing after the rollback err = %v, want not_found", err)
	}
}

func TestGetPing_hidesAnotherUsersPingAndReportsAMissingTable(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ping, err := f.record(t, "mine")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.GetPing(t.Context(), f.pool, ping.ID, userID(t, f.ids)); errs.CodeOf(err) != errs.CodeNotFound {
		t.Fatalf("GetPing as another user err = %v, want not_found", err)
	}
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE system_pings`); err != nil {
		t.Fatal(err)
	}
	if _, err := app.GetPing(t.Context(), f.pool, ping.ID, f.user); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("GetPing without the table err = %v, want internal", err)
	}
}

func TestParseNote_acceptsUpTo140CharactersNotBytes(t *testing.T) {
	t.Parallel()
	for raw, ok := range map[string]bool{
		"":                       true,
		strings.Repeat("é", 140): true,
		strings.Repeat("a", 141): false,
		strings.Repeat("é", 141): false,
	} {
		n, err := domain.ParseNote(raw)
		switch {
		case ok && (err != nil || n.String() != raw):
			t.Errorf("ParseNote(%d runes) = %q, %v, want it kept", len([]rune(raw)), n, err)
		case !ok && errs.CodeOf(err) != errs.CodeInvalidInput:
			t.Errorf("ParseNote(%d runes) err = %v, want invalid_input", len([]rune(raw)), err)
		}
	}
}
