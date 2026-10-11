package identity_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestDepositAddresses_NotImplementedUntilMultichainDepositsLand(t *testing.T) {
	t.Parallel()
	s := identityScenario(t)
	user := testkit.SeedUser(t, s.DB(), testkit.UserOpts{WithWallet: true})
	s.Given(scenario.AsSeededUser("member", user.ID)).
		When(scenario.Get("/v1/me/deposit-addresses")).
		Then(scenario.ExpectProblem(errs.CodeNotImplemented))
}
