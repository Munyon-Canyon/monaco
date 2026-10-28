package ranking_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const callBoardHints = "ranking.hints.call_board_written"

func TestCallBoardTriggers_insertOneTriggerPerEvent(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		handler string
		ev      events.Event
		reason  string
	}{
		"call resolved":    {"ranking.call_board.call_resolved", events.CallResolved{V: 1}, "call_resolved"},
		"vote cast":        {"ranking.call_board.vote_cast", events.VoteCast{V: 1}, "vote_cast"},
		"proposal created": {"ranking.call_board.proposal_created", events.ProposalCreated{V: 1}, "proposal_created"},
		"cabal updated":    {"ranking.call_board.cabal_updated", events.CabalUpdated{V: 1}, "cabal_updated"},
		"profile updated": {
			"ranking.call_board.profile_updated",
			events.UserProfileUpdated{V: 1, Fields: []string{"display_name"}},
			"profile_updated",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newDeliverer(t)
			id := d.event(t, tc.ev)
			d.clock.Advance(time.Minute)
			if duplicate, err := d.deliverAs(t.Context(), t, id, tc.handler, tc.ev); err != nil || duplicate {
				t.Fatalf("first delivery = duplicate %t, %v; want it to succeed", duplicate, err)
			}
			if duplicate, err := d.deliverAs(t.Context(), t, id, tc.handler, tc.ev); err != nil || !duplicate {
				t.Fatalf("redelivery = duplicate %t, %v; want a duplicate", duplicate, err)
			}
			if got := d.callBoardTriggers(t); len(got) != 1 || got[0] != tc.reason+" true" {
				t.Fatalf("call_board_triggers = %q, want one %q stamped at the delivery time", got, tc.reason)
			}
		})
	}
}

func TestCallBoardTriggers_ignoreProfileEditsThatNeverShowOnTheBoard(t *testing.T) {
	t.Parallel()
	d := newDeliverer(t)
	ev := events.UserProfileUpdated{V: 1, Fields: []string{"bio"}}
	id := d.event(t, ev)
	if _, err := d.deliverAs(t.Context(), t, id, "ranking.call_board.profile_updated", ev); err != nil {
		t.Fatal(err)
	}
	if got := d.callBoardTriggers(t); len(got) != 0 {
		t.Fatalf("call_board_triggers = %q, want none for a bio edit", got)
	}
}

func (d deliverer) callBoardTriggers(t *testing.T) []string {
	t.Helper()
	var got []string
	err := d.pool.QueryRow(t.Context(), `SELECT coalesce(array_agg(reason || ' ' || (created_at = $1)::text ORDER BY id), '{}')
		FROM call_board_triggers`, d.clock.Now()).
		Scan(&got)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

type soloCards struct {
	user  ids.UserID
	cabal ids.CabalID
}

func (c soloCards) UsersByID(context.Context, []ids.UserID) (map[ids.UserID]identity.UserCard, error) {
	return map[ids.UserID]identity.UserCard{c.user: {ID: c.user, Handle: "kai", DisplayName: "Kai"}}, nil
}

func (c soloCards) AllCabals(context.Context) ([]cabalport.CabalView, error) {
	return []cabalport.CabalView{{ID: c.cabal, Name: "Pilates"}}, nil
}

func (soloCards) MembersOf(context.Context, []ids.CabalID) (map[ids.CabalID][]cabalport.MemberView, error) {
	return map[ids.CabalID][]cabalport.MemberView{}, nil
}

func seedFiveResolvedCalls(t *testing.T, d deliverer, user, cabal uuid.UUID, at time.Time) {
	t.Helper()
	for i := range 5 {
		proposal := uuid.NewSHA1(uuid.Nil, fmt.Appendf(nil, "board/%d", i))
		for _, q := range []struct {
			sql  string
			args []any
		}{
			{
				`INSERT INTO calls (proposal_id, cabal_id, kind, symbol, opened_at, decided_at, outcome, resolve_at,
				resolved_at, state, updated_at) VALUES ($1, $2, 'buy', 'AAPLx', $3, $3, 'passed', $3, $3, 'resolved', $3)`,
				[]any{proposal, cabal, at},
			},
			{
				`INSERT INTO call_voters (proposal_id, user_id, cabal_id, choice, choice_at, is_right)
				VALUES ($1, $2, $3, 'yes', $4, true)`,
				[]any{proposal, user, cabal, at},
			},
			{
				`INSERT INTO cabal_calls (proposal_id, cabal_id, side, is_right) VALUES ($1, $2, 'yes', true)`,
				[]any{proposal, cabal},
			},
		} {
			if _, err := d.pool.Exec(t.Context(), q.sql, q.args...); err != nil {
				t.Fatalf("seed call %d: %v", i, err)
			}
		}
	}
}

func boardPoller(t *testing.T, d deliverer) (adapters.CallBoardPoller, context.Context) {
	t.Helper()
	m, ctx := boardModule(t, d)
	return adapters.CallBoardPoller{Reads: sqlc.New(d.pool), Runner: m.RunCallBoard(), Clock: d.clock}, ctx
}

func boardModule(t *testing.T, d deliverer) (*ranking.Module, context.Context) {
	t.Helper()
	named := func(name string) uuid.UUID { return uuid.NewSHA1(uuid.Nil, []byte("board/"+name)) }
	user, cabal := ids.UserIDFrom(named("kai")), ids.CabalIDFrom(named("pilates"))
	seedFiveResolvedCalls(t, d, user.UUID(), cabal.UUID(), d.clock.Now().Add(-time.Hour))
	cards := soloCards{user: user, cabal: cabal}
	deps := module.Deps{Pool: d.pool, UoW: d.uow, IDs: d.gen, Clock: d.clock, Bus: d.conn}
	m := ranking.New(deps, ranking.WithPorts(app.Ports{Users: cards, Cabals: cards}))
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorSystem, ID: "poller.ranking.call_board"})
	return m, ctx
}

func TestCallBoardPoller_aResolvedCallWakesTheBoardWithinTwoSeconds(t *testing.T) {
	t.Parallel()
	d := newDeliverer(t)
	board, ctx := boardPoller(t, d)
	if _, err := board.Tick(ctx); err != nil || d.boardRuns(t) != 1 {
		t.Fatalf("first Tick = %v with %d runs, want 1", err, d.boardRuns(t))
	}

	resolved := events.CallResolved{V: 1}
	id := d.event(t, resolved)
	if _, err := d.deliverAs(ctx, t, id, "ranking.call_board.call_resolved", resolved); err != nil {
		t.Fatal(err)
	}
	if _, err := board.Tick(ctx); err != nil || d.boardRuns(t) != 1 {
		t.Fatalf("Tick in the debounce window = %v with %d runs, want it to wait", err, d.boardRuns(t))
	}
	d.clock.Advance(2 * time.Second)
	if report, err := board.Tick(ctx); err != nil || report.Changed != 6 {
		t.Fatalf("Tick after 2 s = %+v, %v; want a run that wrote Kai and Pilates on three periods", report, err)
	}
	if got, triggers := d.boardRuns(t), d.callBoardTriggers(t); got != 2 || len(triggers) != 0 {
		t.Fatalf("runs %d and triggers %q, want 2 runs and the trigger consumed", got, triggers)
	}
}

func TestCallBoardPoller_writesTheRowsAndTheEventInOneTransaction(t *testing.T) {
	t.Parallel()
	d := newDeliverer(t)
	board, ctx := boardPoller(t, d)
	if _, err := board.Tick(ctx); err != nil {
		t.Fatalf("Tick = %v", err)
	}

	var rows, written, sameRun int
	err := d.pool.QueryRow(t.Context(), `SELECT
		(SELECT count(*) FROM call_board_entries WHERE board = 'people' AND period = 'all' AND rank = 1
			AND subject_name = 'Kai' AND rate_bps = 10000),
		(SELECT count(*) FROM events WHERE type = 'ranking.call_board_written'),
		(SELECT count(*) FROM events AS e JOIN call_board_runs AS r ON e.aggregate_id = r.run_id
			WHERE e.type = 'ranking.call_board_written' AND r.rows_written = 6)`).Scan(&rows, &written, &sameRun)
	if err != nil || rows != 1 || written != 1 || sameRun != 1 {
		t.Fatalf(
			"Kai at #1 rows %d, events %d, events for the run %d, err %v; want 1, 1 and 1",
			rows,
			written,
			sameRun,
			err,
		)
	}
}

func (d deliverer) boardRuns(t *testing.T) int {
	t.Helper()
	var n int
	if err := d.pool.QueryRow(t.Context(), `SELECT count(*) FROM call_board_runs`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCallBoardHints_publishOneHintPerRun(t *testing.T) {
	t.Parallel()
	nats := testkit.NATS(t)
	d := newDeliverer(t)
	d.conn = nats.Conn
	sub := testkit.SubscribeCore(t, nats, "hint."+boardsHint)
	run, done := uuid.Nil, d.clock.Now()
	ev := events.RankingCallBoardWritten{V: 1, RunID: run, ComputedAt: done}
	id := d.event(t, ev)

	if duplicate, err := d.deliverAs(t.Context(), t, id, callBoardHints, ev); err != nil || duplicate {
		t.Fatalf("first delivery = duplicate %t, %v; want it to succeed", duplicate, err)
	}
	var got struct {
		RunID      uuid.UUID `json:"run_id"`
		ComputedAt time.Time `json:"computed_at"`
	}
	if err := json.Unmarshal(sub.Next(t), &got); err != nil || got.RunID != run || !got.ComputedAt.Equal(done) {
		t.Fatalf("hint = %+v, %v; want run %s computed at %s", got, err, run, done)
	}
	wantNoHint(t, nats.Conn, sub)
	if duplicate, err := d.deliverAs(t.Context(), t, id, callBoardHints, ev); err != nil || !duplicate {
		t.Fatalf("redelivery = duplicate %t, %v; want a duplicate", duplicate, err)
	}
	wantNoHint(t, nats.Conn, sub)
}

func TestCallBoards_leaderReadsTheBoardTheJobWrote(t *testing.T) {
	t.Parallel()
	d := newDeliverer(t)
	m, ctx := boardModule(t, d)
	if _, err := m.RunCallBoard().Run(ctx, d.clock.Now()); err != nil {
		t.Fatalf("Run = %v", err)
	}
	leader, err := m.CallBoards().CallLeader(ctx, domain.BoardPeople, domain.PeriodAll)
	if err != nil || leader.Name != "Kai" || leader.RateBps != 10000 {
		t.Fatalf("CallLeader = %+v, %v; want Kai at 10000", leader, err)
	}
}
