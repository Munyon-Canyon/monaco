package flows

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	callBoardPoller = "ranking.call_board"
	boardCalls      = 14
	closedCalls     = 12
)

type boardPerson struct {
	name            string
	right, resolved int
}

type boardFixture struct {
	people map[string]uuid.UUID
	closed time.Time
	events int
}

func boardPeople() []boardPerson {
	named := []boardPerson{
		{"Christelle", 12, 13},
		{"Logan", 10, 11},
		{"Nia", 9, 10},
		{"Omar", 8, 9},
		{"Pia", 7, 8},
		{"Quin", 6, 7},
		{"Kai", 11, 14},
		{"Andres", 7, 9},
	}
	people := make([]boardPerson, 0, 24)
	people = append(people, named...)
	for i := range 16 {
		people = append(people, boardPerson{fmt.Sprintf("Rest%02d", i), 3, 5})
	}
	return people
}

func closedPeople() []boardPerson {
	return []boardPerson{
		{"Christelle", 11, 12}, {"Logan", 9, 12}, {"Nia", 7, 12}, {"Omar", 5, 6}, {"Pia", 2, 5},
	}
}

func (defined) SeedsF19b() map[string]Seeder {
	return map[string]Seeder{
		"F19bRunCallBoardOK":                seedSignedInWith(seedResolvedCalls),
		"F19bRunCallBoardCrashBeforeCommit": seedSignedInWith(seedResolvedCalls),
	}
}

func F19bRunCallBoardOK(s *scenario.Scenario) {
	fx := seedCallBoard(s, time.Now().UTC())
	s.Given().When(
		scenario.AwaitTickPastTimeouts(callBoardPoller),
	).Then(fx.awaitBoard()...)
}

func F19bRunCallBoardCrashBeforeCommit(s *scenario.Scenario) {
	fx := seedCallBoard(s, time.Now().UTC())
	s.Given().When(
		scenario.TickCrashingAt(callBoardPoller, faultpoint.BeforeCommit),
		scenario.AwaitTickPastTimeouts(callBoardPoller),
	).Then(fx.awaitBoard()...)
}

func seedCallBoard(s *scenario.Scenario, now time.Time) boardFixture {
	s.Helper()
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	fx := boardFixture{people: map[string]uuid.UUID{}, closed: month.AddDate(0, -2, 0)}
	cabal := testkit.NewCabal(seedT{s}, s.DB()).ID.UUID()
	for _, p := range boardPeople() {
		user := testkit.SeedUser(seedT{s}, s.DB(), testkit.UserOpts{
			Handle: strings.ToLower(p.name) + "_" + ids.Real{}.NewV7().String()[24:30],
		})
		fx.people[p.name] = user.ID.UUID()
	}
	span := now.Sub(month)
	fx.seedCalls(s, cabal, boardPeople(), boardCalls, func(i int) (time.Time, time.Time) {
		return month, month.Add(span * time.Duration(i+1) / (boardCalls + 1)).Truncate(time.Microsecond)
	})
	fx.seedCalls(s, cabal, closedPeople(), closedCalls, func(i int) (time.Time, time.Time) {
		opened := fx.closed.Add(time.Duration(i) * time.Minute)
		return opened, opened.Add(25 * time.Hour)
	})
	fx.events = fx.startTheDay(s)
	return fx
}

func (fx boardFixture) seedCalls(
	s *scenario.Scenario, cabal uuid.UUID, people []boardPerson, n int, when func(i int) (opened, resolved time.Time),
) {
	s.Helper()
	g := ids.Real{}
	var batch pgx.Batch
	for i := range n {
		proposal := g.NewV7()
		opened, resolved := when(i)
		batch.Queue(`INSERT INTO calls (proposal_id, cabal_id, kind, symbol, opened_at, decided_at, outcome,
			market_ref_micros, ref_price_micros, ref_source, resolve_at, resolved_at, end_price_micros, move_bps, state,
			updated_at)
			VALUES ($1, $2, 'buy', 'TSLAx', $3, $3, 'passed', 100000000, 100000000, 'market', $4, $4, 110000000, 1000,
			'resolved', $4)`, proposal, cabal, opened, resolved)
		batch.Queue(`INSERT INTO cabal_calls (proposal_id, cabal_id, side, is_right) VALUES ($1, $2, 'yes', true)`,
			proposal, cabal)
		for _, p := range people {
			if i < p.resolved {
				batch.Queue(`INSERT INTO call_voters (proposal_id, user_id, cabal_id, choice, choice_at, is_right)
					VALUES ($1, $2, $3, 'yes', $4, $5)`, proposal, fx.people[p.name], cabal, opened, i < p.right)
			}
		}
	}
	results := s.DB().SendBatch(s.Context(), &batch)
	defer func() { _ = results.Close() }()
	for range batch.Len() {
		if _, err := results.Exec(); err != nil {
			s.Fatalf("flows: seed the call board fixture: %v", err)
		}
	}
}

func (fx boardFixture) startTheDay(s *scenario.Scenario) int {
	s.Helper()
	var written int
	if err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM events WHERE type = $1`,
		string(events.TypeRankingCallBoardWritten)).Scan(&written); err != nil {
		s.Fatalf("flows: count ranking.call_board_written events: %v", err)
	}
	if _, err := s.DB().Exec(s.Context(), `WITH runs AS (DELETE FROM call_board_runs),
		triggers AS (DELETE FROM call_board_triggers)
		DELETE FROM call_board_baselines WHERE day = (now() AT TIME ZONE 'UTC')::date`); err != nil {
		s.Fatalf("flows: start the board's day over: %v", err)
	}
	return written
}

func (fx boardFixture) awaitBoard() []scenario.Step {
	return []scenario.Step{fx.awaitRanks(), fx.awaitBaselines(), fx.awaitClosedMonth(), fx.awaitBoardEvent()}
}

func (fx boardFixture) awaitRanks() scenario.Step {
	return scenario.Eventually(
		"the month board to rank 24 people with Kai #7 and Andres #8",
		func(s *scenario.Scenario) bool {
			var ranked, kai, andres, same int
			if err := s.DB().QueryRow(s.Context(), `SELECT count(*) FILTER (WHERE rank IS NOT NULL),
			coalesce(max(rank) FILTER (WHERE subject_id = $1), 0), coalesce(max(rank) FILTER (WHERE subject_id = $2), 0),
			count(*) FILTER (WHERE movement_kind = 'same')
			FROM call_board_entries WHERE board = 'people' AND period = 'month' AND subject_id = ANY($3)`,
				fx.people["Kai"], fx.people["Andres"], fx.ids()).Scan(&ranked, &kai, &andres, &same); err != nil {
				s.Fatalf("flows: read the month board: %v", err)
			}
			if ranked == 0 {
				return false
			}
			if ranked != 24 || kai != 7 || andres != 8 || same != 24 {
				s.Fatalf("flows: month board has %d ranked, Kai #%d, Andres #%d, %d unmoved; want 24, #7, #8, 24",
					ranked, kai, andres, same)
			}
			return true
		},
	)
}

func (fx boardFixture) awaitBaselines() scenario.Step {
	return scenario.Eventually("the day's baselines", func(s *scenario.Scenario) bool {
		var got int
		if err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM call_board_baselines
			WHERE board = 'people' AND period = 'month' AND day = (now() AT TIME ZONE 'UTC')::date
			AND subject_id = ANY($1)`, fx.ids()).Scan(&got); err != nil {
			s.Fatalf("flows: read the baselines: %v", err)
		}
		if got > 24 {
			s.Fatalf("flows: %d baselines for 24 ranked people", got)
		}
		return got == 24
	})
}

func (fx boardFixture) awaitClosedMonth() scenario.Step {
	return scenario.Eventually("the closed month with Christelle on top", func(s *scenario.Scenario) bool {
		var top string
		var right, resolved, rows int
		if err := s.DB().
			QueryRow(s.Context(), `SELECT count(*), coalesce(max(subject_id::text) FILTER (WHERE rank = 1), ''),
			coalesce(max(right_count) FILTER (WHERE rank = 1), 0), coalesce(max(resolved) FILTER (WHERE rank = 1), 0)
			FROM call_board_closed WHERE board = 'people' AND period = 'month' AND period_start = $1::date
			AND subject_id = ANY($2)`, fx.closed, fx.ids()).
			Scan(&rows, &top, &right, &resolved); err != nil {
			s.Fatalf("flows: read the closed month: %v", err)
		}
		if rows == 0 {
			return false
		}
		if rows != 5 || top != fx.people["Christelle"].String() || right != 11 || resolved != 12 {
			s.Fatalf("flows: closed month has %d rows, #1 %s at %d of %d; want 5, Christelle at 11 of 12",
				rows, top, right, resolved)
		}
		return true
	})
}

func (fx boardFixture) awaitBoardEvent() scenario.Step {
	return scenario.Eventually("one ranking.call_board_written event", func(s *scenario.Scenario) bool {
		var got int
		if err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM events WHERE type = $1`,
			string(events.TypeRankingCallBoardWritten)).Scan(&got); err != nil {
			s.Fatalf("flows: count ranking.call_board_written events: %v", err)
		}
		got -= fx.events
		if got > 1 {
			s.Fatalf("flows: %d ranking.call_board_written events, want 1", got)
		}
		return got == 1
	})
}

func (fx boardFixture) ids() []uuid.UUID {
	out := make([]uuid.UUID, 0, len(fx.people))
	for _, id := range fx.people {
		out = append(out, id)
	}
	return out
}
