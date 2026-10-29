package app_test

import (
	"testing"
	"time"

	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func seedSeptember(t *testing.T, pool rig, cards *boardCards) {
	t.Helper()
	cabal := cards.addCabal("Pilates", cabalport.StatusActive)
	seedPeople(t, pool, cards, cabal, time.Date(2026, 9, 7, 9, 0, 0, 0, time.UTC), []personSeed{
		{name: "Christelle", right: 11, resolved: 12},
		{name: "Logan", right: 9, resolved: 12},
		{name: "Nia", right: 7, resolved: 12},
		{name: "Omar", right: 5, resolved: 6},
		{name: "Pia", right: 2, resolved: 5},
	})
}

func closedMonthRows(t *testing.T, pool rig, board string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM call_board_closed WHERE board = $1 AND period = 'month'`,
		board).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func wantTops(t *testing.T, pool rig, name string, n int) {
	t.Helper()
	tops, err := app.NewCallBoards(pool).
		ClosedTops(t.Context(), boardUser(name).UUID(), domain.BoardPeople, domain.PeriodMonth)
	if err != nil || len(tops) != n {
		t.Fatalf("ClosedTops(%s) = %+v, %v; want %d", name, tops, err, n)
	}
	if n > 0 && (tops[0].Right != 11 || tops[0].Resolved != 12 ||
		!tops[0].PeriodStart.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))) {
		t.Fatalf("ClosedTops(%s) = %+v; want September at 11 of 12", name, tops)
	}
}

func TestRunCallBoard_closesAMonthSixHoursAfterItEndsAndOnlyOnce(t *testing.T) {
	t.Parallel()
	pool, cards := newRig(t), &boardCards{}
	seedSeptember(t, pool, cards)
	run := newBoardRun(t, pool, cards)

	runBoardAt(t, run, time.Date(2026, 10, 1, 5, 59, 0, 0, time.UTC))
	if n := closedMonthRows(t, pool, "people"); n != 0 {
		t.Fatalf("closed month rows before 06:00 = %d, want none", n)
	}
	runBoardAt(t, run, time.Date(2026, 10, 1, 6, 0, 0, 0, time.UTC))
	people, cabals := closedMonthRows(t, pool, "people"), closedMonthRows(t, pool, "cabals")
	if people != 5 || cabals != 1 {
		t.Fatalf("closed month rows = %d people and %d cabals, want 5 and 1", people, cabals)
	}
	runBoardAt(t, run, time.Date(2026, 10, 1, 6, 5, 0, 0, time.UTC))
	if n := closedMonthRows(t, pool, "people"); n != people {
		t.Fatalf("closed month rows after a second run = %d, want %d", n, people)
	}

	wantTops(t, pool, "Christelle", 1)
	wantTops(t, pool, "Logan", 0)
	wantTops(t, pool, "Pia", 0)
}

func TestRunCallBoard_keepsAClosedBoardAsItWasClosed(t *testing.T) {
	t.Parallel()
	pool, cards := newRig(t), &boardCards{}
	seedSeptember(t, pool, cards)
	run := newBoardRun(t, pool, cards)
	runBoardAt(t, run, time.Date(2026, 10, 1, 7, 0, 0, 0, time.UTC))
	execAll(t, pool, `UPDATE call_voters SET is_right = NOT is_right WHERE user_id = $1`,
		boardUser("Christelle").UUID())

	runBoardAt(t, run, time.Date(2026, 10, 2, 7, 0, 0, 0, time.UTC))

	wantTops(t, pool, "Christelle", 1)
}

func TestRunCallBoard_failsWhenTheHistoryItKeepsCannotBeReadOrWritten(t *testing.T) {
	t.Parallel()
	for name, statements := range map[string][]string{
		"the closed boards cannot be listed": {"ALTER TABLE call_board_closed RENAME TO call_board_closed_gone"},
		"the closed boards refuse rows": {
			"ALTER TABLE call_board_closed ADD CONSTRAINT refuse CHECK (false) NOT VALID",
		},
		"the baselines refuse rows": {
			"ALTER TABLE call_board_baselines ADD CONSTRAINT refuse CHECK (false) NOT VALID",
		},
		"the run record refuses rows":   {"ALTER TABLE call_board_runs ADD CONSTRAINT refuse CHECK (false) NOT VALID"},
		"the last calls cannot be read": {"ALTER TABLE calls RENAME COLUMN symbol TO symbol_gone"},
		"the old baselines cannot be pruned": {
			`CREATE FUNCTION refuse_prune() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'no'; END $$`,
			`CREATE TRIGGER refuse BEFORE DELETE ON call_board_baselines FOR EACH STATEMENT EXECUTE FUNCTION refuse_prune()`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pool, cards := newRig(t), &boardCards{}
			seedSeptember(t, pool, cards)
			for _, statement := range statements {
				execAll(t, pool, statement)
			}
			if _, err := newBoardRun(
				t,
				pool,
				cards,
			).Run(pollerContext(t), time.Date(2026, 10, 2, 7, 0, 0, 0, time.UTC)); err == nil {
				t.Fatal("Run error = nil, want the failure")
			}
		})
	}
}

func TestRunCallBoard_failsWhenTodaysBaselinesCannotBeRead(t *testing.T) {
	t.Parallel()
	pool, cards := newRig(t), &boardCards{}
	seedSeptember(t, pool, cards)
	run := newBoardRun(t, pool, cards)
	runBoardAt(t, run, time.Date(2026, 10, 2, 7, 0, 0, 0, time.UTC))
	execAll(t, pool, "ALTER TABLE call_board_baselines RENAME TO call_board_baselines_gone")
	if _, err := app.NewRunCallBoard(pool, pool.uow, testkit.NewIDs(7), clock.Real{}, cards, cards).
		Run(pollerContext(t), time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("Run error = nil, want the baseline read to fail")
	}
}

func TestCallBoards_closedTopsPassesTheReadErrorOn(t *testing.T) {
	t.Parallel()
	pool := newRig(t)
	execAll(t, pool, "ALTER TABLE call_board_closed RENAME TO call_board_closed_gone")
	if _, err := app.NewCallBoards(pool).ClosedTops(
		t.Context(), boardUser("Kai").UUID(), domain.BoardPeople, domain.PeriodMonth,
	); err == nil {
		t.Fatal("ClosedTops error = nil, want the read to fail")
	}
}
