package ranking_test

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	namesHandler = "ranking.names"
	boardsHint   = "global.leaderboards_updated"
)

type boards struct {
	run   uuid.UUID
	done  time.Time
	alice uuid.UUID
	bob   uuid.UUID
	cabal uuid.UUID
}

func seedBoards(t *testing.T, d deliverer, withRun bool) boards {
	t.Helper()
	b := boards{run: d.gen.NewV7(), done: d.clock.Now(), alice: d.gen.NewV7(), bob: d.gen.NewV7(), cabal: d.gen.NewV7()}
	if withRun {
		older := sqlc.InsertLeaderboardRunParams{
			RunID: d.gen.NewV7(), AsOf: b.done, PricesAsOf: b.done,
			StartedAt: b.done.Add(-2 * time.Hour), FinishedAt: b.done.Add(-time.Hour),
		}
		latest := older
		latest.RunID, latest.StartedAt, latest.FinishedAt = b.run, b.done.Add(-time.Second), b.done
		for _, run := range []sqlc.InsertLeaderboardRunParams{older, latest} {
			if err := sqlc.New(d.pool).InsertLeaderboardRun(t.Context(), run); err != nil {
				t.Fatal(err)
			}
		}
	}
	_, err := d.pool.Exec(t.Context(), `INSERT INTO leaderboard_entries (
		board, range, rank, subject_id, subject_name, subject_handle, subject_picture_url, subject_created_at,
		value_micros, pnl_micros, prices_as_of, computed_at
	) VALUES
		('people', 'ALL', 1, $1, 'alice', 'alice', NULL, $4, 1, 0, $4, $4),
		('people', '1D', 1, $1, 'alice', 'alice', NULL, $4, 1, 0, $4, $4),
		('people', 'ALL', 2, $2, 'bob', 'bob', NULL, $4, 1, 0, $4, $4),
		('cabal_members:' || $3::text, 'ALL', 1, $1, 'alice', 'alice', NULL, $4, 1, 0, $4, $4),
		('cabals', 'ALL', 1, $1, 'alice', NULL, NULL, $4, 1, 0, $4, $4)`,
		b.alice, b.bob, b.cabal, b.done)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func (d deliverer) entries(t *testing.T) []string {
	t.Helper()
	var got []string
	err := d.pool.QueryRow(t.Context(), `SELECT coalesce(array_agg(concat_ws(' ', split_part(board, ':', 1), range,
		subject_name, coalesce(subject_handle, '-'), coalesce(subject_picture_url, '-'))
		ORDER BY board, range, rank), '{}') FROM leaderboard_entries`).Scan(&got)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func (d deliverer) revs(t *testing.T) []int {
	t.Helper()
	var got []int
	err := d.pool.QueryRow(t.Context(),
		`SELECT coalesce(array_agg(rev ORDER BY finished_at), '{}') FROM leaderboard_runs`).Scan(&got)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func renamed(user uuid.UUID, fields ...string) events.UserProfileUpdated {
	return events.UserProfileUpdated{
		V: 1, UserID: user, Fields: fields, Handle: "quill", DisplayName: "Quillen",
		PhotoURL: "https://cdn.example.com/q.png",
	}
}

func wantNoHint(t *testing.T, conn *bus.Conn, sub *testkit.CoreSubscription) {
	t.Helper()
	conn.PublishHint(t.Context(), boardsHint, []byte("end"))
	if got := string(sub.Next(t)); got != "end" {
		t.Fatalf("a hint %s arrived; want none", got)
	}
}

func wantHint(t *testing.T, conn *bus.Conn, sub *testkit.CoreSubscription, b boards) {
	t.Helper()
	var got struct {
		RunID      uuid.UUID `json:"run_id"`
		ComputedAt time.Time `json:"computed_at"`
	}
	if err := json.Unmarshal(sub.Next(t), &got); err != nil || got.RunID != b.run || !got.ComputedAt.Equal(b.done) {
		t.Fatalf("hint = %+v, %v; want run %s computed at %s", got, err, b.run, b.done)
	}
	wantNoHint(t, conn, sub)
}

func renamedBoards() []string {
	return []string{
		"cabal_members ALL Quillen quill https://cdn.example.com/q.png",
		"cabals ALL alice - -",
		"people 1D Quillen quill https://cdn.example.com/q.png",
		"people ALL Quillen quill https://cdn.example.com/q.png",
		"people ALL bob bob -",
	}
}

func TestNames_renamesThePeopleAndMembersBoardsOnceAndBumpsTheLatestRun(t *testing.T) {
	t.Parallel()
	nats := testkit.NATS(t)
	d := newDeliverer(t)
	d.conn = nats.Conn
	b := seedBoards(t, d, true)
	sub := testkit.SubscribeCore(t, nats, "hint."+boardsHint)
	ev := renamed(b.alice, "display_name", "handle")
	id := d.event(t, ev)

	if duplicate, err := d.deliverAs(t.Context(), t, id, namesHandler, ev); err != nil || duplicate {
		t.Fatalf("first delivery = duplicate %t, %v; want it to succeed", duplicate, err)
	}
	if got := d.entries(t); !slices.Equal(got, renamedBoards()) {
		t.Fatalf("entries = %q, want %q", got, renamedBoards())
	}
	if got := d.revs(t); !slices.Equal(got, []int{0, 1}) {
		t.Fatalf("revs = %v, want only the latest run bumped once", got)
	}
	wantHint(t, nats.Conn, sub, b)

	if duplicate, err := d.deliverAs(t.Context(), t, id, namesHandler, ev); err != nil || !duplicate {
		t.Fatalf("redelivery = duplicate %t, %v; want a duplicate", duplicate, err)
	}
	again := d.event(t, ev)
	if duplicate, err := d.deliverAs(t.Context(), t, again, namesHandler, ev); err != nil || duplicate {
		t.Fatalf("second event = duplicate %t, %v; want it to succeed", duplicate, err)
	}
	if got := d.revs(t); !slices.Equal(got, []int{0, 1}) || !slices.Equal(d.entries(t), renamedBoards()) {
		t.Fatalf("revs after a second event = %v; want nothing changed", got)
	}
	wantNoHint(t, nats.Conn, sub)
}

func TestNames_changesNothingWithoutABoardChange(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		user   func(boards) uuid.UUID
		fields []string
	}{
		"a field no board shows": {user: func(b boards) uuid.UUID { return b.alice }, fields: []string{"bio"}},
		"a user on no board":     {user: func(boards) uuid.UUID { return uuid.Max }, fields: []string{"display_name"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			nats := testkit.NATS(t)
			d := newDeliverer(t)
			d.conn = nats.Conn
			b := seedBoards(t, d, true)
			before := d.entries(t)
			sub := testkit.SubscribeCore(t, nats, "hint."+boardsHint)
			ev := renamed(tc.user(b), tc.fields...)

			duplicate, err := d.deliverAs(t.Context(), t, d.event(t, ev), namesHandler, ev)
			if err != nil || duplicate {
				t.Fatalf("delivery = duplicate %t, %v; want it to succeed", duplicate, err)
			}
			if got := d.entries(t); !slices.Equal(got, before) || !slices.Equal(d.revs(t), []int{0, 0}) {
				t.Fatalf("entries = %q, revs %v; want nothing changed", got, d.revs(t))
			}
			wantNoHint(t, nats.Conn, sub)
		})
	}
}

func TestNames_publishesNoHintBeforeTheFirstRun(t *testing.T) {
	t.Parallel()
	nats := testkit.NATS(t)
	d := newDeliverer(t)
	d.conn = nats.Conn
	b := seedBoards(t, d, false)
	sub := testkit.SubscribeCore(t, nats, "hint."+boardsHint)
	ev := renamed(b.alice, "photo_url")

	if duplicate, err := d.deliverAs(t.Context(), t, d.event(t, ev), namesHandler, ev); err != nil || duplicate {
		t.Fatalf("delivery = duplicate %t, %v; want it to succeed", duplicate, err)
	}
	if got := d.revs(t); len(got) != 0 {
		t.Fatalf("revs = %v, want no run", got)
	}
	wantNoHint(t, nats.Conn, sub)
}

func TestNames_retriesWhenATableIsUnreadable(t *testing.T) {
	t.Parallel()
	for _, table := range []string{"leaderboard_entries", "leaderboard_runs"} {
		t.Run(table, func(t *testing.T) {
			t.Parallel()
			d := newDeliverer(t)
			b := seedBoards(t, d, true)
			if _, err := d.pool.Exec(t.Context(), `ALTER TABLE `+table+` RENAME TO gone`); err != nil {
				t.Fatal(err)
			}
			ev := renamed(b.alice, "display_name")
			if _, err := d.deliverAs(t.Context(), t, d.event(t, ev), namesHandler, ev); err == nil {
				t.Fatalf("delivery with %s missing succeeded; want an error", table)
			}
		})
	}
}
