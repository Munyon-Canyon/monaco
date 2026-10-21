package flows

import (
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	withdrawEVMTo   = "0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed"
	withdrawEVMBody = `{"amount_micros":"2000000","destination_kind":"evm","network":"base","to_address":"` + withdrawEVMTo + `"}`
)

func (defined) StepsF30WithdrawEVM() FlowStep {
	return FlowStep{
		Flow: "30", Order: 5, Name: "EVM withdrawal", Owner: "#4972", Status: StepReady, Script: F30WithdrawEVMOK,
	}
}

func F30WithdrawEVMOK(s *scenario.Scenario) {
	member := SeedWithdrawer(s)
	s.Given(scenario.AsSeededUser("member", member.ID)).
		When(
			AuthorizeWithdrawal(withdrawEVMBody),
			WithdrawAuthorized(withdrawEVMBody),
			scenario.ExpectStatus(http.StatusAccepted),
			scenario.ExpectJSON("status", "submitted"),
			scenario.Remember("withdrawal_id", "withdrawal"),
			scenario.AwaitTick(withdrawPoller),
			scenario.AwaitTick(withdrawPoller),
			scenario.Get(withdrawPath+"/{withdrawal}"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("status", "confirmed"),
			scenario.ExpectJSON("to_address", withdrawEVMTo),
			SettledWithdrawal(member),
		).
		Then(scenario.ExpectEvents(events.TypeWithdrawalSubmitted, 1), scenario.ExpectEvents(events.TypeWithdrawalConfirmed, 1))
}
