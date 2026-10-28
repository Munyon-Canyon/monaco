package ranking_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func TestFlow19a_ResolveCalls_OK(t *testing.T) {
	t.Parallel()
	flows.F19aResolveCallsOK(flow19Scenario(t))
}

func TestCalls_aRetractedVoteLeavesNoChoiceAndAnOlderCastDoesNotBringItBack(t *testing.T) {
	t.Parallel()
	w := newCallsWorld(t, 0)
	cast := func(at time.Time) {
		w.d.callsDeliver(t, w.ports, "ranking.calls.vote_cast", events.VoteCast{
			V: 1, ProposalID: w.proposal, CabalID: w.cabal, VoterID: w.bob, Choice: "yes", CastAt: at,
		})
	}
	cast(w.opened.Add(time.Minute))
	w.d.callsDeliver(t, w.ports, "ranking.calls.vote_retracted", events.VoteRetracted{
		V: 1, ProposalID: w.proposal, CabalID: w.cabal, VoterID: w.bob, Choice: "yes",
		RetractedAt: w.opened.Add(2 * time.Minute),
	})
	cast(w.opened.Add(time.Minute))
	if choice, _ := w.d.voter(t, w.proposal, w.bob); choice != "<nil>" {
		t.Fatalf("choice after the retraction and an older redelivery = %s, want none", choice)
	}
	cast(w.opened.Add(3 * time.Minute))
	if choice, _ := w.d.voter(t, w.proposal, w.bob); choice != "yes" {
		t.Fatalf("choice after a newer cast = %s, want yes", choice)
	}
}

type fakeResolver struct {
	reports []app.ResolveReport
	err     error
	runs    int
}

func (f *fakeResolver) Run(context.Context, time.Time, int32) (app.ResolveReport, error) {
	f.runs++
	if f.err != nil {
		return app.ResolveReport{}, f.err
	}
	return f.reports[min(f.runs-1, len(f.reports)-1)], nil
}

func TestCallsResolverPoller_tickDrainsFullBatchesAndStopsOnAShortOne(t *testing.T) {
	t.Parallel()
	r := &fakeResolver{reports: []app.ResolveReport{
		{Scanned: app.CallsBatch, Resolved: app.CallsBatch - 1, Voided: 1},
		{Scanned: 3, Resolved: 2, Voided: 1},
	}}
	p := adapters.CallsResolverPoller{Resolver: r, Clock: clock.Real{}}
	got, err := p.Tick(t.Context())
	if err != nil || r.runs != 2 || got.Scanned != app.CallsBatch+3 || got.Changed != app.CallsBatch+3 {
		t.Fatalf("Tick() = %+v, %v after %d runs, want both batches counted", got, err, r.runs)
	}
	if p.Name() != "ranking.calls_resolver" || p.Interval() != time.Minute {
		t.Fatalf("poller = %s every %v, want ranking.calls_resolver every minute", p.Name(), p.Interval())
	}
}

func TestCallsResolverPoller_tickStopsWhenAFullBatchChangesNothing(t *testing.T) {
	t.Parallel()
	r := &fakeResolver{reports: []app.ResolveReport{{Scanned: app.CallsBatch}}}
	got, err := adapters.CallsResolverPoller{Resolver: r, Clock: clock.Real{}}.Tick(t.Context())
	if err != nil || r.runs != 1 || got.Changed != 0 {
		t.Fatalf("Tick() = %+v, %v after %d runs, want one run", got, err, r.runs)
	}
}

func TestCallsResolverPoller_tickReturnsTheResolverError(t *testing.T) {
	t.Parallel()
	want := errs.New(errs.CodeInternal, "test.resolver")
	r := &fakeResolver{err: want}
	if _, err := (adapters.CallsResolverPoller{Resolver: r, Clock: clock.Real{}}).Tick(
		t.Context(),
	); !errors.Is(
		err,
		want,
	) {
		t.Fatalf("Tick() error = %v, want %v", err, want)
	}
}
