package treasury_test

import (
	"net/http"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

type fundingReads struct {
	readsOnly
	m *funding.Module
}

func (f fundingReads) Balances() fundingport.Balances { return f.m.Balances() }

func (f fundingReads) PausesIn(tx db.Tx) fundingport.Pauses { return f.m.PausesIn(tx) }

func TestFundRoutes_areWiredToTheCabalAndFundingModules(t *testing.T) {
	t.Parallel()
	s := scenario.New(t, scenario.WithModules(
		func(d module.Deps) module.Module { return treasury.New(d) },
		func(d module.Deps) module.Module { return fundingReads{readsOnly{"funding"}, funding.New(d)} },
		func(d module.Deps) module.Module { return cabalReads{readsOnly{"cabal"}, d} },
		func(d module.Deps) module.Module { return identityReads{readsOnly{"identity"}, d} },
	))
	c := testkit.NewCabal(t, s.DB())
	stranger := testkit.SeedUser(t, s.DB(), testkit.UserOpts{})
	s.Given(scenario.AsSeededUser("stranger", stranger.ID)).
		When(
			scenario.Post("/v1/cabals/"+c.ID.String()+"/fund", `{"amount_micros":"5000000"}`),
		).
		Then(scenario.ExpectStatus(http.StatusForbidden), scenario.ExpectProblem(errs.CodeNotCabalMember))
}
