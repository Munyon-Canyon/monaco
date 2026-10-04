package governance_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (d voteDB) expiry() *app.ExpiryPoller {
	return app.NewExpiryPoller(d.uow, d.pool, d.clk, app.NoHints{})
}

func (d voteDB) tick(ctx context.Context) (poller.Report, error) {
	return d.expiry().Tick(observability.WithActor(ctx, "system:poller.governance.proposal_expiry"))
}

func (d voteDB) mustTick(t *testing.T, scanned, changed int) {
	t.Helper()
	got, err := d.tick(t.Context())
	if err != nil || got.Scanned != scanned || got.Changed != changed {
		t.Fatalf("Tick = %+v, %v, want scanned %d changed %d", got, err, scanned, changed)
	}
}

func (d voteDB) expiring(t *testing.T, at time.Time) uuid.UUID {
	t.Helper()
	p := d.buy(d.ids.NewV7())
	p.ExpiresAt = at
	d.insert(t, p)
	return p.ID
}

func (d voteDB) status(t *testing.T, id uuid.UUID) string {
	t.Helper()
	return d.row(t, id).status.String
}

func (d voteDB) refuse(t *testing.T, table, column string, id uuid.UUID) {
	t.Helper()
	for _, stmt := range []string{
		`CREATE FUNCTION refuse_` + table + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
			IF NEW.` + column + ` = '` + id.String() + `' THEN RAISE EXCEPTION 'refused'; END IF;
			RETURN NEW; END $$`,
		`CREATE TRIGGER refuse BEFORE INSERT OR UPDATE ON ` + table +
			` FOR EACH ROW EXECUTE FUNCTION refuse_` + table + `()`,
	} {
		if _, err := d.pool.Exec(t.Context(), stmt); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExpiry_expiresOpenProposalsOnceTheirDeadlineIsReached(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	now := d.clk.Now()
	due, onTheDot := d.expiring(t, now.Add(-time.Minute)), d.expiring(t, now)
	later := d.expiring(t, now.Add(time.Minute))
	decided := d.expiring(t, now.Add(-time.Hour))
	d.transition(t, decided, "open", "passed", "")
	d.mustTick(t, 2, 2)
	for id, want := range map[uuid.UUID]string{due: "expired", onTheDot: "expired", later: "open", decided: "passed"} {
		if got := d.status(t, id); got != want {
			t.Errorf("status of %s = %s, want %s", id, got, want)
		}
	}
	if r := d.row(t, due); !r.updatedAt.Equal(now) {
		t.Errorf("updated_at = %s, want the tick's clock %s", r.updatedAt, now)
	}
	expired := d.payloads(t, due, events.TypeProposalExpired)
	if len(expired) != 1 || d.terminal(t, decided) != 0 {
		t.Fatalf("proposal.expired payloads = %s, want one for the due proposal and none for the decided one", expired)
	}
	d.mustTick(t, 0, 0)
	d.clk.Advance(time.Minute)
	d.mustTick(t, 1, 1)
	if got := d.status(t, later); got != "expired" || d.terminal(t, later) != 1 {
		t.Fatalf("status of the later proposal = %s, want expired with one terminal event", got)
	}
}

func TestExpiry_takesAtMostOneBatchPerTick(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	for range app.ExpiryBatch + 1 {
		d.expiring(t, d.clk.Now())
	}
	d.mustTick(t, app.ExpiryBatch, app.ExpiryBatch)
	d.mustTick(t, 1, 1)
}

func TestExpiry_aRefusedProposalDoesNotHoldBackTheRest(t *testing.T) {
	t.Parallel()
	for _, table := range []struct{ name, column string }{{"proposals", "id"}, {"events", "aggregate_id"}} {
		t.Run(table.name, func(t *testing.T) {
			t.Parallel()
			d := newVoteDB(t)
			stuck := d.expiring(t, d.clk.Now().Add(-time.Hour))
			fine := d.expiring(t, d.clk.Now())
			d.refuse(t, table.name, table.column, stuck)
			got, err := d.tick(t.Context())
			if err == nil || got.Scanned != 2 || got.Changed != 1 {
				t.Fatalf("Tick = %+v, %v, want an error after scanning 2 and expiring 1", got, err)
			}
			if d.status(t, stuck) != "open" || d.status(t, fine) != "expired" || d.terminal(t, stuck) != 0 {
				t.Fatal("want the refused proposal still open with no event, and the other expired")
			}
		})
	}
}

func TestExpiry_aTickThatCannotReadFails(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.tick(ctx); err == nil {
		t.Fatal("Tick on a cancelled context succeeded")
	}
}

func (d voteDB) waitingOnLocks(t *testing.T, n int) {
	t.Helper()
	testkit.Eventually(t, func() bool {
		var waiting int
		err := d.pool.QueryRow(t.Context(), `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&waiting)
		return err == nil && waiting == n
	}, 10*time.Second)
}

func (d voteDB) holdBallots(t *testing.T) (release func()) {
	t.Helper()
	gate, err := d.pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(gate.Release)
	for _, stmt := range []string{
		`SELECT pg_advisory_lock(583)`,
		`CREATE FUNCTION hold_ballot() RETURNS trigger LANGUAGE plpgsql
			AS $$ BEGIN PERFORM pg_advisory_xact_lock_shared(583); RETURN NEW; END $$`,
		`CREATE TRIGGER hold BEFORE INSERT ON votes FOR EACH ROW EXECUTE FUNCTION hold_ballot()`,
	} {
		if _, err := gate.Exec(t.Context(), stmt); err != nil {
			t.Fatal(err)
		}
	}
	return func() {
		if _, err := gate.Exec(t.Context(), `SELECT pg_advisory_unlock(583)`); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExpiry_RaceWithVote(t *testing.T) {
	t.Parallel()
	t.Run("the vote decides first", raceVoteFirst)
	t.Run("the expiry tick goes first", raceExpiryFirst)
}

func raceVoteFirst(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.open(t, 1)
	d.clk.Set(p.ExpiresAt)
	release := d.holdBallots(t)
	h := d.handler(threshold{rule: domain.RuleMajority})
	var voted, ticked errgroup.Group
	var vote app.CastVoteResult
	var tick poller.Report
	voted.Go(func() (err error) {
		vote, err = d.cast(t.Context(), h, p.ID, v[0], domain.ChoiceYes)
		return err
	})
	d.waitingOnLocks(t, 1)
	ticked.Go(func() (err error) {
		tick, err = d.tick(t.Context())
		return err
	})
	d.waitingOnLocks(t, 2)
	release()
	if err := voted.Wait(); err != nil || vote.Status != domain.StatusPassed {
		t.Fatalf("vote = %+v, %v, want passed", vote, err)
	}
	if err := ticked.Wait(); err != nil || tick.Scanned != 1 || tick.Changed != 0 {
		t.Fatalf("tick = %+v, %v, want the proposal scanned and skipped", tick, err)
	}
	if got := d.status(t, p.ID); got != "passed" || d.terminal(t, p.ID) != 1 {
		t.Fatalf("status %s, want passed with exactly one terminal event", got)
	}
}

func raceExpiryFirst(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.open(t, 1)
	d.clk.Set(p.ExpiresAt)
	d.mustTick(t, 1, 1)
	_, err := d.cast(t.Context(), d.handler(threshold{rule: domain.RuleMajority}), p.ID, v[0], domain.ChoiceYes)
	if errs.CodeOf(err) != errs.CodeProposalClosed {
		t.Fatalf("a vote after the expiry err = %v, want proposal_closed", err)
	}
	if got := d.status(t, p.ID); got != "expired" || d.terminal(t, p.ID) != 1 {
		t.Fatalf("status %s, want expired with exactly one terminal event", got)
	}
}
