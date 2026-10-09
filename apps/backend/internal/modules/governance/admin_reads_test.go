package governance_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	adminport "github.com/monaco/monaco/apps/backend/internal/modules/admin/port"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
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
		reads := app.NewAdminReads(app.NewProposalReads(d.pool, fakes.NewTrading()), tc, tc, tc, tc)
		if _, err := reads.Proposal(t.Context(), ids.ProposalIDFrom(p.ID)); errs.CodeOf(err) != errs.CodeDBUnavailable {
			t.Errorf("%s: Proposal = %v, want db_unavailable", name, err)
		}
	}
	failing := stubReads{cabal: boom}
	reads := app.NewAdminReads(app.NewProposalReads(d.pool, fakes.NewTrading()), failing, failing, failing, failing)
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
