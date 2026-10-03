package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type nudgeRig struct {
	pool   *pgxpool.Pool
	clock  *testkit.Clock
	uow    *db.UnitOfWork
	poller *app.EmitNudges
}

func newNudgeRig(t *testing.T) nudgeRig {
	t.Helper()
	pool := testkit.DB(t)
	g := testkit.NewIDs(testkit.RandSeed(t))
	c := testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond))
	uow := db.New(pool, g, c)
	return nudgeRig{pool: pool, clock: c, uow: uow, poller: app.NewEmitNudges(uow, pool, c, time.Hour)}
}

func nudgeCtx(t *testing.T) context.Context {
	t.Helper()
	return auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorSystem, ID: "poller.identity.nudges"})
}

func (r nudgeRig) tick(t *testing.T) poller.Report {
	t.Helper()
	report, err := r.poller.Tick(nudgeCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func nudgesFor(t *testing.T, pool *pgxpool.Pool, user uuid.UUID) []events.UserNudgeDue {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT payload FROM events WHERE type = $1 AND aggregate_id = $2 ORDER BY payload->>'at'`,
		string(events.TypeUserNudgeDue), user)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []events.UserNudgeDue
	for rows.Next() {
		var payload []byte
		var ev events.UserNudgeDue
		if err := rows.Scan(&payload); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(payload, &ev); err != nil {
			t.Fatal(err)
		}
		got = append(got, ev)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return got
}

func nudgeEventCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE type = $1`,
		string(events.TypeUserNudgeDue)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestEmitNudges_isTheIdentityNudgesPollerAtItsInterval(t *testing.T) {
	t.Parallel()
	p := app.NewEmitNudges(nil, nil, clock.Real{}, 24*time.Hour)
	if p.Name() != "identity.nudges" || p.Interval() != 24*time.Hour {
		t.Fatalf("poller = %s every %s, want identity.nudges every 24h", p.Name(), p.Interval())
	}
}

func TestEmitNudges_nudgesAStuckUserOnDays1And8And15ThenStops(t *testing.T) {
	t.Parallel()
	r := newNudgeRig(t)
	user := seedNudgeRow(t, r.pool, r.clock.Now(), nudgeRow{state: "AWAITING_PHONE"})
	start := r.clock.Now()
	var days []int
	for day := 1; day <= 30; day++ {
		r.clock.Advance(24 * time.Hour)
		if r.tick(t).Changed == 1 {
			days = append(days, day)
		}
	}
	if !slices.Equal(days, []int{1, 8, 15}) {
		t.Fatalf("nudged on days %v, want [1 8 15]", days)
	}
	got := nudgesFor(t, r.pool, user)
	want := make([]events.UserNudgeDue, 0, len(days))
	for i, day := range days {
		want = append(want, events.UserNudgeDue{
			V: 1, UserID: user, Kind: "add_phone", NudgeNumber: i + 1,
			At: start.Add(time.Duration(day) * 24 * time.Hour),
		})
	}
	for i := range got {
		got[i].At = got[i].At.UTC()
	}
	if !slices.Equal(got, want) {
		t.Fatalf("nudges = %+v, want %+v", got, want)
	}
}

func TestEmitNudges_skipsSuspendedBannedDeletedAndFinishedUsers(t *testing.T) {
	t.Parallel()
	r := newNudgeRig(t)
	now, settled := r.clock.Now(), 2*24*time.Hour
	socials := seedNudgeRow(t, r.pool, now, nudgeRow{state: "AWAITING_SOCIALS", changedAgo: settled})
	for _, row := range []nudgeRow{
		{state: "AWAITING_PHONE", status: "suspended", changedAgo: settled},
		{state: "AWAITING_PHONE", status: "banned", changedAgo: settled},
		{state: "AWAITING_SOCIALS", status: "deleted", changedAgo: settled},
		{state: "ONBOARDING_COMPLETED", changedAgo: settled},
		{state: "CREATED", changedAgo: settled},
	} {
		seedNudgeRow(t, r.pool, now, row)
	}
	if report := r.tick(t); report.Scanned != 1 || report.Changed != 1 {
		t.Fatalf("report = %+v, want 1 scanned and 1 nudged", report)
	}
	got := nudgesFor(t, r.pool, socials)
	if len(got) != 1 || got[0].Kind != "link_x" || got[0].NudgeNumber != 1 {
		t.Fatalf("nudges for the socials user = %+v, want one link_x nudge", got)
	}
	if n := nudgeEventCount(t, r.pool); n != 1 {
		t.Fatalf("user.nudge_due events = %d, want 1", n)
	}
}

func TestEmitNudges_aStateChangeStartsTheCadenceAgain(t *testing.T) {
	t.Parallel()
	r := newNudgeRig(t)
	user := seedNudgeRow(t, r.pool, r.clock.Now(), nudgeRow{
		state: "AWAITING_PHONE", changedAgo: 30 * 24 * time.Hour,
		nudgedAgo: 8 * 24 * time.Hour, count: 3,
	})
	if report := r.tick(t); report.Changed != 0 {
		t.Fatalf("report = %+v, want no nudge after three", report)
	}
	userID, err := ids.ParseUserID(user.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		return adapters.Users{}.UpdateAuthState(ctx, tx.Queries(), userID, domain.AuthAwaitingPhone,
			domain.AuthAwaitingSocials, r.clock.Now())
	}); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(23 * time.Hour)
	if report := r.tick(t); report.Changed != 0 {
		t.Fatalf("report = %+v, want no nudge inside the first day", report)
	}
	r.clock.Advance(time.Hour)
	if report := r.tick(t); report.Changed != 1 {
		t.Fatalf("report = %+v, want one nudge a day after the change", report)
	}
	got := nudgesFor(t, r.pool, user)
	if len(got) != 1 || got[0].Kind != "link_x" || got[0].NudgeNumber != 1 {
		t.Fatalf("nudges = %+v, want one link_x nudge numbered 1", got)
	}
}

func TestEmitNudges_pagesThroughMoreUsersThanOnePage(t *testing.T) {
	t.Parallel()
	r := newNudgeRig(t)
	total := app.NudgePage + 1
	if _, err := r.pool.Exec(t.Context(), `INSERT INTO users
		(id, privy_user_id, login_provider, auth_state, auth_state_changed_at, created_at, updated_at)
		SELECT gen_random_uuid(), 'did:privy:page-' || n, 'sms', 'AWAITING_PHONE', $1, $1, $1
		FROM generate_series(1, $2::int) AS n`, r.clock.Now().Add(-48*time.Hour), total); err != nil {
		t.Fatal(err)
	}
	if report := r.tick(t); report.Scanned != total || report.Changed != total {
		t.Fatalf("report = %+v, want %d scanned and nudged", report, total)
	}
	if n := nudgeEventCount(t, r.pool); n != total {
		t.Fatalf("user.nudge_due events = %d, want %d", n, total)
	}
	if report := r.tick(t); report.Scanned != 0 || report.Changed != 0 {
		t.Fatalf("second tick = %+v, want nothing due", report)
	}
}

func TestEmitNudges_aFailedReadEndsTheTick(t *testing.T) {
	t.Parallel()
	r := newNudgeRig(t)
	ctx, cancel := context.WithCancel(nudgeCtx(t))
	cancel()
	if _, err := r.poller.Tick(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Tick = %v, want context.Canceled", err)
	}
}

func TestEmitNudges_aFailedPageAppendsNoNudge(t *testing.T) {
	t.Parallel()
	r := newNudgeRig(t)
	user := seedNudgeRow(t, r.pool, r.clock.Now(), nudgeRow{state: "AWAITING_PHONE", changedAgo: 48 * time.Hour})
	if _, err := r.pool.Exec(t.Context(), `
		CREATE FUNCTION refuse_nudge() RETURNS trigger LANGUAGE plpgsql AS
		$$ BEGIN RAISE EXCEPTION 'nudge refused'; END $$;
		CREATE TRIGGER refuse_nudge BEFORE UPDATE OF last_nudged_at ON users
		FOR EACH ROW EXECUTE FUNCTION refuse_nudge();`); err != nil {
		t.Fatal(err)
	}
	report, err := r.poller.Tick(nudgeCtx(t))
	if err == nil || report.Scanned != 1 || report.Changed != 0 {
		t.Fatalf("Tick = %+v, %v, want 1 scanned, none nudged and the update's error", report, err)
	}
	if got := nudgesFor(t, r.pool, user); len(got) != 0 {
		t.Fatalf("nudges = %+v, want none", got)
	}
}
