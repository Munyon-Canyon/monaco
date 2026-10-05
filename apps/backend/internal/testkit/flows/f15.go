package flows

import (
	"crypto/ed25519"
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	withdrawPath      = "/v1/me/withdrawals"
	withdrawElsewhere = "9xQeWvG816bUx9EPjHmaT23yvVMvM9fQj4a8PHF4H6P"
	withdrawBody      = `{"amount_micros":"2000000","to_address":"` + withdrawElsewhere + `"}`
	withdrawPoller    = "funding.withdrawals"
)

func SeedWithdrawer(s *scenario.Scenario) testkit.SeededUser {
	member := testkit.SeedUser(seedT{s}, s.DB(), testkit.UserOpts{WithWallet: true})
	member.Address = chain.AddressOf(fakes.PrivyWalletKey(member.PrivyWalletID).Public().(ed25519.PublicKey))
	if _, err := s.DB().Exec(s.Context(), `UPDATE user_wallets SET address = $1 WHERE user_id = $2`,
		string(member.Address), member.ID.UUID()); err != nil {
		s.Fatalf("flows: give the member the fake Privy wallet's address: %v", err)
	}
	return member
}

func SettledWithdrawal(member testkit.SeededUser) scenario.Step {
	return func(s *scenario.Scenario) {
		count := func() int {
			var n int
			if err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM user_txns
				WHERE user_id = $1 AND kind = 'withdrawal' AND status = 'settled'`, member.ID.UUID()).Scan(&n); err != nil {
				s.Fatalf("flows: count settled withdrawals: %v", err)
			}
			return n
		}
		scenario.Eventually("a settled withdrawal in the user ledger", func(*scenario.Scenario) bool {
			return count() > 0
		})(s)
		if n := count(); n != 1 {
			s.Fatalf("flows: settled withdrawals = %d, want 1", n)
		}
	}
}

func F15WithdrawOK(s *scenario.Scenario) {
	member := SeedWithdrawer(s)
	s.Given(scenario.AsSeededUser("member", member.ID), scenario.FakeUpstream(finalizedStatus())).
		When(
			scenario.Post(withdrawPath, withdrawBody),
			scenario.ExpectStatus(http.StatusAccepted),
			scenario.ExpectJSON("status", "submitted"),
			scenario.Remember("withdrawal_id", "withdrawal"),
			scenario.AwaitTick(withdrawPoller),
			scenario.AwaitTick(withdrawPoller),
			scenario.Get(withdrawPath+"/{withdrawal}"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("status", "confirmed"),
			scenario.ExpectJSON("amount_micros", "2000000"),
			SettledWithdrawal(member),
		).
		Then(scenario.ExpectEvents(events.TypeWithdrawalSubmitted, 1))
}

func F15WithdrawCrashBeforeCommit(s *scenario.Scenario) {
	member := SeedWithdrawer(s)
	s.Given(scenario.AsSeededUser("member", member.ID), scenario.FakeUpstream(finalizedStatus())).
		When(
			scenario.Post(withdrawPath, withdrawBody),
			scenario.Retry(),
			scenario.ExpectStatus(http.StatusAccepted),
			scenario.AwaitTick(withdrawPoller),
			scenario.AwaitTick(withdrawPoller),
			SettledWithdrawal(member),
		).
		Then(scenario.ExpectEvents(events.TypeWithdrawalSubmitted, 1))
}

func F15WithdrawInvalidInput(s *scenario.Scenario) {
	refuseWithdrawal(s, `{"amount_micros":"999999","to_address":"`+withdrawElsewhere+`"}`, errs.CodeInvalidInput)
}

func F15WithdrawInvalidAddress(s *scenario.Scenario) {
	refuseWithdrawal(s, `{"amount_micros":"2000000","to_address":"not-a-solana-address"}`, errs.CodeInvalidAddress)
}

func F15WithdrawWithdrawToOwnWallet(s *scenario.Scenario) {
	member := SeedWithdrawer(s)
	s.Given(scenario.AsSeededUser("member", member.ID)).
		When(scenario.Post(withdrawPath, `{"amount_micros":"2000000","to_address":"`+string(member.Address)+`"}`)).
		Then(
			scenario.ExpectProblem(errs.CodeWithdrawToOwnWallet),
			scenario.ExpectEvents(events.TypeWithdrawalSubmitted, 0),
		)
}

func F15WithdrawInsufficientFunds(s *scenario.Scenario) {
	refuseWithdrawal(s, `{"amount_micros":"900000000000","to_address":"`+withdrawElsewhere+`"}`,
		errs.CodeInsufficientFunds)
}

func F15WithdrawPrivyUnavailable(s *scenario.Scenario) {
	member := SeedWithdrawer(s)
	s.Given(
		scenario.AsSeededUser("member", member.ID),
		scenario.FakeUpstream(fakes.Step{
			Route: "/privy/v1/wallets/" + member.PrivyWalletID + "/rpc", Action: fakes.ActionFail, Status: 503,
			Times: 100, Reset: true,
		}),
	).
		When(scenario.Post(withdrawPath, withdrawBody)).
		Then(
			scenario.ExpectProblem(errs.CodePrivyUnavailable),
			scenario.ExpectEvents(events.TypeWithdrawalSubmitted, 0),
		)
}

func refuseWithdrawal(s *scenario.Scenario, body string, code errs.Code) {
	member := SeedWithdrawer(s)
	s.Given(scenario.AsSeededUser("member", member.ID)).
		When(scenario.Post(withdrawPath, body)).
		Then(scenario.ExpectProblem(code), scenario.ExpectEvents(events.TypeWithdrawalSubmitted, 0))
}
