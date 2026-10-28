package governance_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestPort_talliesCountBallotsAndLeaveUnknownProposalsOut(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	tallies := governance.New(module.Deps{Pool: d.pool}).Tallies()
	p := d.buy(d.ids.NewV7(), d.ids.NewV7(), d.ids.NewV7(), d.ids.NewV7())
	d.insert(t, p)
	d.ballot(t, p.ID, p.VoterIds[0], "yes")
	d.ballot(t, p.ID, p.VoterIds[1], "yes")
	d.ballot(t, p.ID, p.VoterIds[2], "no")
	known, unknown := ids.ProposalIDFrom(p.ID), ids.ProposalIDFrom(d.ids.NewV7())

	got, err := tallies.Tallies(t.Context(), []ids.ProposalID{known, unknown})

	want := port.Tally{
		Yes: 2, No: 1, Voters: 4, Needed: 3, ExpiresAt: p.ExpiresAt, Status: domain.StatusOpen, Kind: "buy",
	}
	if err != nil || len(got) != 1 || got[known] != want {
		t.Fatalf("Tallies = %+v, %v, want only %+v", got, err, want)
	}
}

func TestPort_talliesRefuseAnUnknownStoredStatus(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	tallies := governance.New(module.Deps{Pool: d.pool}).Tallies()
	p := d.buy(d.ids.NewV7())
	d.insert(t, p)
	d.transition(t, p.ID, "open", "gremlins", "")
	_, err := tallies.Tallies(t.Context(), []ids.ProposalID{ids.ProposalIDFrom(p.ID)})
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("Tallies err = %v, want decode_failed", err)
	}
}

func TestPort_talliesFailWithDBUnavailableOnEitherRead(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	tallies := governance.New(module.Deps{Pool: d.pool}).Tallies()
	id := []ids.ProposalID{ids.ProposalIDFrom(d.ids.NewV7())}
	if _, err := d.pool.Exec(t.Context(), "ALTER TABLE votes RENAME TO votes_gone"); err != nil {
		t.Fatal(err)
	}
	if _, err := tallies.Tallies(t.Context(), id); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Errorf("Tallies with the ballot read failing err = %v, want db_unavailable", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := tallies.Tallies(ctx, id); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Errorf("Tallies on a cancelled context err = %v, want db_unavailable", err)
	}
}
