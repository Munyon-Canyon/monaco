package governance_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func withGovernance() scenario.Option {
	return scenario.WithModules(func(d module.Deps) module.Module {
		return governance.New(d, governance.WithPorts(governance.Ports{Cabals: cabal.New(d).Queries()}))
	})
}

func TestFlow10_CastVote_OK(t *testing.T) {
	t.Parallel()
	flows.F10CastVoteOK(scenario.New(t, withGovernance()))
}

func TestProposalHints_MemberReceives_NonMemberDoesNot(t *testing.T) {
	t.Parallel()
	flows.F10CastVoteOK(scenario.New(t, withGovernance()))
}

func TestFlow10_CastVote_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F10CastVoteUnauthorized(scenario.New(t, withGovernance()))
}

func TestFlow10_CastVote_ProposalNotFound(t *testing.T) {
	t.Parallel()
	flows.F10CastVoteProposalNotFound(scenario.New(t, withGovernance()))
}

func TestFlow10_CastVote_NotAVoter(t *testing.T) {
	t.Parallel()
	flows.F10CastVoteNotAVoter(scenario.New(t, withGovernance()))
}

func TestFlow10_CastVote_ProposalClosed(t *testing.T) {
	t.Parallel()
	flows.F10CastVoteProposalClosed(scenario.New(t, withGovernance()))
}

func TestCastVote_overHTTPAProposalWhoseCabalIsGoneIsCabalNotFound(t *testing.T) {
	t.Parallel()
	s := scenario.New(t, withGovernance())
	d := proposalDB{q: sqlc.New(s.DB()), ids: testkit.NewIDs(7), now: clock.Real{}.Now().UTC()}
	voter, err := ids.ParseUserID(d.ids.NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	p := d.buy(voter.UUID())
	d.insert(t, p)
	s.Given(scenario.AsSeededUser("alice", voter)).
		When(scenario.Post("/v1/proposals/"+p.ID.String()+"/votes", `{"choice":"yes"}`)).
		Then(scenario.ExpectProblem(errs.CodeCabalNotFound))
}
