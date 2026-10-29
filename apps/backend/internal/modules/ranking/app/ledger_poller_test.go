package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type ledgerPollerFake struct {
	pending  []uuid.UUID
	voters   []uuid.UUID
	members  []cabalport.MemberView
	failOn   string
	gotLimit int32
	gotIDs   []uuid.UUID
}

func (f *ledgerPollerFake) fail(at string) error {
	if f.failOn == at {
		return errs.New(errs.CodeInternal, at)
	}
	return nil
}

func (f *ledgerPollerFake) AllCabals(context.Context) ([]cabalport.CabalView, error) {
	return []cabalport.CabalView{{ID: ids.CabalIDFrom(ids.Real{}.NewV7())}}, f.fail("cabals")
}

func (f *ledgerPollerFake) MembersOf(
	_ context.Context, in []ids.CabalID,
) (map[ids.CabalID][]cabalport.MemberView, error) {
	return map[ids.CabalID][]cabalport.MemberView{in[0]: f.members}, f.fail("members")
}

func (f *ledgerPollerFake) ResolvedVotersBetween(
	context.Context, sqlc.ResolvedVotersBetweenParams,
) ([]uuid.UUID, error) {
	return f.voters, f.fail("voters")
}

func (f *ledgerPollerFake) PendingLedgerUsers(
	_ context.Context, arg sqlc.PendingLedgerUsersParams,
) ([]uuid.UUID, error) {
	f.gotLimit, f.gotIDs = arg.BatchLimit, arg.CandidateIds
	return f.pending, f.fail("pending")
}

func (f *ledgerPollerFake) builder(build app.LedgerBuild, inputs func() app.LedgerInputs) app.BuildLedgers {
	b := app.NewBuildLedgers(f, nil, inputs, f, testkit.NewClock(time.Date(2026, 10, 25, 17, 0, 0, 0, time.UTC)))
	b.Build = build
	return b
}

func TestLedgerDelivery_isTheLatestSundayFivePMUTC(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ now, want time.Time }{
		"a minute before":   {time.Date(2026, 10, 25, 16, 59, 0, 0, time.UTC), time.Date(2026, 10, 18, 17, 0, 0, 0, time.UTC)},
		"on the dot":        {time.Date(2026, 10, 25, 17, 0, 0, 0, time.UTC), time.Date(2026, 10, 25, 17, 0, 0, 0, time.UTC)},
		"Monday morning":    {time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC), time.Date(2026, 10, 25, 17, 0, 0, 0, time.UTC)},
		"Saturday night":    {time.Date(2026, 10, 24, 23, 0, 0, 0, time.UTC), time.Date(2026, 10, 18, 17, 0, 0, 0, time.UTC)},
		"another time zone": {time.Date(2026, 10, 25, 12, 30, 0, 0, time.FixedZone("x", -5*3600)), time.Date(2026, 10, 25, 17, 0, 0, 0, time.UTC)},
	} {
		if got := app.LedgerDelivery(tc.now); !got.Equal(tc.want) {
			t.Errorf("%s: LedgerDelivery(%s) = %s, want %s", name, tc.now, got, tc.want)
		}
	}
}

func TestBuildLedgers_failuresOfTheReadsStopTheBatch(t *testing.T) {
	t.Parallel()
	member := cabalport.MemberView{UserID: ids.UserIDFrom(ids.Real{}.NewV7())}
	for _, at := range []string{"cabals", "members", "voters", "pending"} {
		t.Run(at, func(t *testing.T) {
			t.Parallel()
			f := &ledgerPollerFake{
				failOn: at, members: []cabalport.MemberView{member}, pending: []uuid.UUID{ids.Real{}.NewV7()},
			}
			_, err := f.builder(app.BuildLedger, func() app.LedgerInputs { return app.LedgerInputs{} }).Run(t.Context())
			if err == nil {
				t.Fatalf("Run with %s failing returned no error", at)
			}
		})
	}
}

func TestBuildLedgers_nothingPendingReadsNoWindow(t *testing.T) {
	t.Parallel()
	member := cabalport.MemberView{UserID: ids.UserIDFrom(ids.Real{}.NewV7())}
	f := &ledgerPollerFake{members: []cabalport.MemberView{member, member}, voters: []uuid.UUID{member.UserID.UUID()}}
	report, err := f.builder(app.BuildLedger, func() app.LedgerInputs {
		t.Fatal("read the inputs with nothing pending")
		return app.LedgerInputs{}
	}).Run(t.Context())
	if err != nil || report != (app.LedgerReport{}) {
		t.Fatalf("Run = %+v, %v; want an empty report", report, err)
	}
	if f.gotLimit != app.LedgerBatch || len(f.gotIDs) != 1 {
		t.Fatalf("pending asked for limit %d over %d candidates, want %d over 1 deduplicated",
			f.gotLimit, len(f.gotIDs), app.LedgerBatch)
	}
}

type failingTreasury struct{ app.Treasury }

func (failingTreasury) MemberStakesAt(context.Context, time.Time) ([]treasury.MemberStake, error) {
	return nil, errs.New(errs.CodeInternal, "stakes")
}

func TestBuildLedgers_aWindowThatCannotLoadFailsTheRun(t *testing.T) {
	t.Parallel()
	f := &ledgerPollerFake{pending: []uuid.UUID{ids.Real{}.NewV7()}}
	inputs := func() app.LedgerInputs { return app.LedgerInputs{Ports: app.Ports{Treasury: failingTreasury{}}} }
	if _, err := f.builder(app.BuildLedger, inputs).Run(t.Context()); err == nil {
		t.Fatal("Run with a failing window returned no error")
	}
}

func TestLedgerPoller_isNamedAndRunsEveryFifteenMinutes(t *testing.T) {
	t.Parallel()
	p := app.LedgerPoller{}
	if p.Name() != "ranking.weekly_ledger" || p.Interval() != 15*time.Minute {
		t.Fatalf("poller = %s every %s, want ranking.weekly_ledger every 15m0s", p.Name(), p.Interval())
	}
}
