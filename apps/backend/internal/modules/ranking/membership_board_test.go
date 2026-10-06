package ranking_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestMembership_theMembersBoardFollowsAJoinAndALeaveWithinASecond(t *testing.T) {
	t.Parallel()
	rig := newValuationRig(t)
	gen, clk := testkit.NewIDs(618), testkit.NewClock(rig.now.Add(time.Minute))
	d := deliverer{pool: rig.pool, gen: gen, clock: clk, uow: db.New(rig.pool, gen, clk)}
	poller := adapters.ValuationPoller{
		Reads: sqlc.New(rig.pool), Runner: app.NewRunValuation(rig.ports, testkit.USDCMint),
		Writer: app.NewSnapshotWriter(d.uow, gen), Clock: clk,
	}
	tick := func() {
		t.Helper()
		if _, err := poller.Tick(observability.WithActor(t.Context(), "system:poller.ranking.valuation")); err != nil {
			t.Fatal(err)
		}
	}
	board := app.MembersBoard(rig.cabal.ID.UUID())
	wantBoard := func(runs, members int) {
		t.Helper()
		if got := count(t, rig.pool, `SELECT count(*) FROM leaderboard_runs`); got != runs {
			t.Fatalf("runs = %d, want %d", got, runs)
		}
		if got := rig.rows(t, board); got != members {
			t.Fatalf("members board has %d rows, want %d", got, members)
		}
	}
	tick()
	wantBoard(1, 1)

	joiner := testkit.SeedUser(t, rig.pool, testkit.UserOpts{})
	if _, err := rig.pool.Exec(t.Context(), `INSERT INTO cabal_members (cabal_id, user_id, role, can_vote, joined_at)
		VALUES ($1, $2, 'member', true, $3)`, rig.cabal.ID.UUID(), joiner.ID.UUID(), rig.now); err != nil {
		t.Fatal(err)
	}
	joined := events.CabalMemberJoined{
		V: 1, CabalID: rig.cabal.ID.UUID(), UserID: joiner.ID.UUID(), Role: "member", Via: "open",
	}
	if _, err := d.deliverAs(t.Context(), t, d.event(t, joined), "ranking.membership", joined); err != nil {
		t.Fatal(err)
	}
	clk.Advance(999 * time.Millisecond)
	tick()
	wantBoard(1, 1)
	clk.Advance(time.Millisecond)
	tick()
	wantBoard(2, 2)
	if got := count(t, rig.pool, `SELECT count(*) FROM leaderboard_entries WHERE board = $1 AND range = 'ALL'
		AND subject_id = $2 AND value_micros = 0 AND return_bps IS NULL`, board, joiner.ID.UUID()); got != 1 {
		t.Fatalf("the new member's row with zero value and no return = %d, want 1", got)
	}

	_, err := rig.pool.Exec(t.Context(), `DELETE FROM cabal_members WHERE cabal_id = $1 AND user_id = $2`,
		rig.cabal.ID.UUID(), joiner.ID.UUID())
	if err != nil {
		t.Fatal(err)
	}
	left := events.CabalMemberLeft{V: 1, CabalID: rig.cabal.ID.UUID(), UserID: joiner.ID.UUID()}
	if _, err := d.deliverAs(t.Context(), t, d.event(t, left), "ranking.membership.left", left); err != nil {
		t.Fatal(err)
	}
	clk.Advance(time.Second)
	tick()
	wantBoard(3, 1)
}
