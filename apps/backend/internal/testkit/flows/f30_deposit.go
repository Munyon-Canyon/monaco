package flows

import (
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	evmForwardPoller   = "funding.evm_deposit_forward"
	f30EVMWalletID     = "privy-evm-deposit-wallet-f30"
	f30EVMAddress      = "0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed"
	f30ForwardedMicros = 27_500_000
)

func (defined) StepsF30Deposit() FlowStep {
	return FlowStep{
		Flow: "30", Order: 1, Name: "EVM deposit credited", Owner: "#4967", Status: StepReady,
		Script: F30EVMDepositCredited,
	}
}

func ForwardedEVMDepositExecution() multichain.Execution {
	return multichain.Execution{
		ID: "privy-execution-f30", ActionID: "privy-action-f30", Network: chain.NetworkBase,
		SourceTx:     "0x8c1f0a3e5b7d9c2e4f6a8b0c1d3e5f7a9b1c3d5e7f9a1b3c5d7e9f1a3b5c7d9e",
		AmountMicros: money.MicrosFromUint64(f30ForwardedMicros), State: multichain.ActionSucceeded,
	}
}

func F30EVMDepositCredited(s *scenario.Scenario) {
	user := seedDepositWallet(s)
	if _, err := s.DB().Exec(s.Context(), `UPDATE user_wallets SET evm_privy_wallet_id = $2, evm_address = $3,
		evm_created_at = now() WHERE user_id = $1`, user.ID.UUID(), f30EVMWalletID, f30EVMAddress); err != nil {
		s.Fatalf("flows: enroll EVM deposit wallet: %v", err)
	}
	s.Given(append(oneInboundTransfer(user), scenario.FakeUpstream(fakes.Step{
		Route: "/rpc/getTokenAccountsByOwner", Action: fakes.ActionSucceed, Times: 100, Reset: true,
	}))...).When(
		scenario.AwaitTick(evmForwardPoller),
		scenario.Get("/v1/me/balance"),
		scenario.ExpectStatus(http.StatusOK),
		scenario.ExpectJSON("arriving_micros", "27500000"),
		scenario.AwaitTick(depositPoller),
		scenario.AwaitTick(depositPoller),
		expectDeposit(user),
		scenario.Eventually("the EVM forward arrived", forwardArrived(user)),
		scenario.AwaitTick(evmForwardPoller),
		scenario.Get("/v1/me/balance"),
		scenario.ExpectStatus(http.StatusOK),
		scenario.ExpectJSON("arriving_micros", "0"),
	).Then(
		scenario.ExpectAllEvents(events.TypeDepositEVMForwarded, 1),
		scenario.ExpectAllEvents(events.TypeDepositCredited, 1),
	)
}

func forwardArrived(user testkit.SeededUser) func(*scenario.Scenario) bool {
	return func(s *scenario.Scenario) bool {
		var arrived bool
		if err := s.DB().QueryRow(s.Context(), `SELECT EXISTS (SELECT 1 FROM evm_deposit_forwards
			WHERE user_id = $1 AND status = 'arrived' AND solana_signature IS NOT NULL)`,
			user.ID.UUID()).Scan(&arrived); err != nil {
			s.Fatalf("flows: read EVM forward: %v", err)
		}
		return arrived
	}
}
