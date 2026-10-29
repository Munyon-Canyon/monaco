package governance_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func TestOpenVoteCounts_CountsOpenProposalsPerCabalAndOmitsTheRest(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	cabal, quiet, unknown := d.ids.NewV7(), d.ids.NewV7(), d.ids.NewV7()
	voter := d.ids.NewV7()
	open := d.proposal(t, cabal, d.now.Add(-2*time.Hour), voter)
	closed := d.proposal(t, cabal, d.now.Add(-time.Hour), voter)
	if n := d.transition(t, closed, "open", "passed", "majority"); n != 1 {
		t.Fatalf("transition = %d rows, want 1", n)
	}
	d.proposal(t, quiet, d.now, voter)
	d.transition(t, d.proposal(t, quiet, d.now, voter), "open", "passed", "majority")

	got, err := d.facts().OpenVoteCounts(t.Context(), []ids.CabalID{
		ids.CabalIDFrom(cabal), ids.CabalIDFrom(quiet), ids.CabalIDFrom(unknown),
	})
	if err != nil || len(got) != 2 || got[ids.CabalIDFrom(cabal)] != 1 || got[ids.CabalIDFrom(quiet)] != 1 {
		t.Fatalf("OpenVoteCounts = %v, %v, want 1 for each of the two cabals with an open proposal", got, err)
	}
	d.ballot(t, open, voter, "yes")
	again, err := d.facts().OpenVoteCounts(t.Context(), []ids.CabalID{ids.CabalIDFrom(cabal)})
	if err != nil || again[ids.CabalIDFrom(cabal)] != 1 {
		t.Fatalf("after the voter's ballot = %v, %v, want the open proposal still counted", again, err)
	}
	none, err := d.facts().OpenVoteCounts(t.Context(), nil)
	if err != nil || len(none) != 0 {
		t.Fatalf("OpenVoteCounts(nil) = %v, %v, want an empty map", none, err)
	}
}

func TestOpenVoteCounts_FailsInternalWhenTheReadFails(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.facts().
		OpenVoteCounts(ctx, []ids.CabalID{ids.CabalIDFrom(d.ids.NewV7())}); errs.CodeOf(
		err,
	) != errs.CodeInternal {
		t.Fatalf("canceled read: err = %v, want internal", err)
	}
}
