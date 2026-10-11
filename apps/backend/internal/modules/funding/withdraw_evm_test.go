package funding_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const evmTo = "0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed"

func TestWithdraw_EVMDestinationIsNotImplementedYet(t *testing.T) {
	t.Parallel()
	s := flow15Scenario(t)
	member := flows.SeedWithdrawer(s)
	s.Given(scenario.AsSeededUser("member", member.ID)).
		When(scenario.Post("/v1/me/withdrawals",
			`{"amount_micros":"2000000","to_address":"`+evmTo+`","destination_kind":"evm","network":"base"}`)).
		Then(scenario.ExpectProblem(errs.CodeNotImplemented),
			scenario.ExpectEvents(events.TypeWithdrawalSubmitted, 0))
}

func TestWithdraw_BadEVMDestinationIsRefusedBeforeAnyHold(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		body string
		want errs.Code
	}{
		"bad checksum": {`{"amount_micros":"2000000","destination_kind":"evm","network":"base",` +
			`"to_address":"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAeD"}`, errs.CodeInvalidAddress},
		"solana address": {`{"amount_micros":"2000000","destination_kind":"evm","network":"base",` +
			`"to_address":"` + string(withdrawTo) + `"}`, errs.CodeInvalidAddress},
		"no network": {
			`{"amount_micros":"2000000","destination_kind":"evm","to_address":"` + evmTo + `"}`,
			errs.CodeInvalidNetwork,
		},
		"solana kind on base": {
			`{"amount_micros":"2000000","network":"base","to_address":"` + evmTo + `"}`,
			errs.CodeInvalidNetwork,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := flow15Scenario(t)
			member := flows.SeedWithdrawer(s)
			s.Given(scenario.AsSeededUser("member", member.ID)).
				When(scenario.Post("/v1/me/withdrawals", c.body)).
				Then(scenario.ExpectProblem(c.want), scenario.ExpectEvents(events.TypeWithdrawalSubmitted, 0))
			var rows int
			if err := s.DB().
				QueryRow(t.Context(), `SELECT count(*) FROM withdrawals`).
				Scan(&rows); err != nil ||
				rows != 0 {
				t.Fatalf("withdrawals = %d, %v, want none", rows, err)
			}
		})
	}
}
