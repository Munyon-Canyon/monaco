package governance_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	adminport "github.com/monaco/monaco/apps/backend/internal/modules/admin/port"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type stubReads struct {
	users   error
	events  error
	actions error
	cabal   error
}

func (s stubReads) UsersByID(context.Context, []ids.UserID) (map[ids.UserID]identityport.UserCard, error) {
	return nil, s.users
}

func (s stubReads) EventsByAggregate(context.Context, string, uuid.UUID, []string, int) ([]bus.EventRow, error) {
	return nil, s.events
}

func (s stubReads) RecentActions(context.Context, string, string, int) ([]adminport.Action, error) {
	return nil, s.actions
}

func (s stubReads) Cabal(context.Context, ids.CabalID) (cabalport.CabalView, error) {
	return cabalport.CabalView{}, s.cabal
}

func TestAdminReads_PassOnEveryPortFailure(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	p := d.buy(d.ids.NewV7())
	d.insert(t, p)
	boom := errs.New(errs.CodeDBUnavailable, "t")
	for name, tc := range map[string]stubReads{
		"the proposer's handle": {users: boom},
		"the status history":    {events: boom},
		"the admin actions":     {actions: boom},
	} {
		reads := app.NewAdminReads(
			app.NewProposalReads(
				d.pool, fakes.NewTrading(), fakes.NewCabal(nil, nil), marketfake.NewCatalog(marketfake.Fixtures()...),
				&cardBook{}, &returnBook{}, &replyBook{},
			),
			tc,
			tc,
			tc,
			tc,
		)
		if _, err := reads.Proposal(t.Context(), ids.ProposalIDFrom(p.ID)); errs.CodeOf(err) != errs.CodeDBUnavailable {
			t.Errorf("%s: Proposal = %v, want db_unavailable", name, err)
		}
	}
	failing := stubReads{cabal: boom}
	reads := app.NewAdminReads(
		app.NewProposalReads(
			d.pool, fakes.NewTrading(), fakes.NewCabal(nil, nil), marketfake.NewCatalog(marketfake.Fixtures()...),
			&cardBook{}, &returnBook{}, &replyBook{},
		),
		failing,
		failing,
		failing,
		failing,
	)
	if _, err := reads.CabalProposals(
		t.Context(),
		ids.CabalIDFrom(p.CabalID),
		app.FilterAll,
		0,
		"",
	); errs.CodeOf(
		err,
	) !=
		errs.CodeDBUnavailable {
		t.Errorf("CabalProposals with a failing cabal read = %v, want db_unavailable", err)
	}
}

func TestAdminProposal_seesEverySide(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	mia, kai := d.ids.NewV7(), d.ids.NewV7()
	p := d.proposedBy(t, d.caller, mia, kai)
	d.ballot(t, p, mia, "yes")
	d.ballot(t, p, kai, "no")
	reads := app.NewAdminReads(d.reads(), stubReads{}, stubReads{}, stubReads{}, stubReads{})
	got, err := reads.Proposal(t.Context(), ids.ProposalIDFrom(p))
	if err != nil {
		t.Fatal(err)
	}
	sides := map[uuid.UUID]domain.Choice{}
	for _, v := range got.Detail.Voters {
		sides[v.UserID.UUID()] = v.Choice
	}
	if sides[mia] != domain.ChoiceYes || sides[kai] != domain.ChoiceNo {
		t.Errorf("admin voters = %v, want every choice", sides)
	}
	member := d.get(t, p, d.caller)
	for _, v := range member.Voters {
		if v.UserID.UUID() != d.caller && v.Choice != "" {
			t.Errorf("member read of the same proposal shows %+v, want other sides redacted", v)
		}
	}
}
