package funding_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestWithdraw_EVMDestinationIsNotImplementedYet(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"evm kind":     `{"amount_micros":"2000000","to_address":"0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed","destination_kind":"evm","network":"base"}`,
		"base network": `{"amount_micros":"2000000","to_address":"0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed","network":"base"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := flow15Scenario(t)
			member := flows.SeedWithdrawer(s)
			s.Given(scenario.AsSeededUser("member", member.ID)).
				When(scenario.Post("/v1/me/withdrawals", body)).
				Then(scenario.ExpectProblem(errs.CodeNotImplemented),
					scenario.ExpectEvents(events.TypeWithdrawalSubmitted, 0))
		})
	}
}
