package flows

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	depositWallet = "5kwEmpcR8Txq1b4bDazRm9j4cx8Qo2aiE53rYA1dCDDP"
	depositPoller = "funding.deposits"
)

func scannedBeforeEveryWallet() time.Time { return time.Unix(-1, 0).UTC() }

func (defined) WorkerEnvF05() []string { return []string{"FUNDING_DEPOSIT_POLL_INTERVAL=2s"} }

func F05CreditDepositOK(s *scenario.Scenario) {
	user := seedDepositWallet(s)
	s.Given(oneInboundTransfer(user)...).When(
		scenario.AwaitTick(depositPoller),
		scenario.AwaitTick(depositPoller),
		expectDeposit(user),
		scenario.EventuallyHints("balance_changed", 2),
		scenario.Get("/v1/me/txns"),
		scenario.ExpectStatus(http.StatusOK),
		scenario.ExpectField("items", oneDeposit),
	).Then()
}

func oneInboundTransfer(user testkit.SeededUser) []scenario.Step {
	return []scenario.Step{
		scenario.AsSeededUser("member", user.ID),
		scenario.FakeUpstream(fakes.Step{
			Route: "/rpc/getSignaturesForAddress", Action: fakes.ActionSucceed,
			Fixture: "/rpc/getSignaturesForAddress", Times: 100, Reset: true,
		}),
		scenario.FakeUpstream(fakes.Step{
			Route: "/rpc/getTransaction", Action: fakes.ActionSucceed,
			Fixture: "/rpc/getTransaction", Times: 100, Reset: true,
		}),
	}
}

func F05CreditDepositCrashBeforeCommit(s *scenario.Scenario) {
	user := seedDepositWallet(s)
	s.Given(oneInboundTransfer(user)...).When(
		scenario.TickCrashingAt(depositPoller, faultpoint.BeforeCommit),
		scenario.AwaitTick(depositPoller),
		scenario.AwaitTick(depositPoller),
		expectDeposit(user),
	).Then(scenario.ExpectAllEvents(events.TypeDepositCredited, 1))
}

func oneDeposit(s *scenario.Scenario, raw json.RawMessage) {
	var items []struct {
		Kind       string `json:"kind"`
		Status     string `json:"status"`
		USDCMicros string `json:"usdc_micros"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		s.Fatalf("flows: decode txns: %v", err)
	}
	if len(items) != 1 || items[0].Kind != "deposit" || items[0].Status != "settled" ||
		items[0].USDCMicros != "27500000" {
		s.Fatalf("flows: txns = %+v, want one settled deposit of 27500000", items)
	}
}

func F05CreditDepositRPCUnavailable(s *scenario.Scenario) {
	seedDepositWallet(s)
	s.Given(scenario.FakeUpstream(fakes.Step{
		Route: "/rpc/getSignaturesForAddress", Action: fakes.ActionFail, Status: 503, Times: 100, Reset: true,
	})).When(
		scenario.AwaitTick(depositPoller),
		scenario.AwaitTick(depositPoller),
		scenario.ExpectTickFailed(depositPoller, string(errs.CodeRPCUnavailable)),
	).Then(scenario.ExpectEvents(events.TypeDepositCredited, 0))
}

func seedDepositWallet(s *scenario.Scenario) testkit.SeededUser {
	user := testkit.SeedUser(seedT{s}, s.DB(), testkit.UserOpts{WithWallet: true})
	user.Address = chain.SolanaAddress(depositWallet)
	if _, err := s.DB().Exec(s.Context(), `DELETE FROM user_wallets WHERE address = $1`, user.Address); err != nil {
		s.Fatalf("flows: clear deposit wallet: %v", err)
	}
	if _, err := s.DB().Exec(
		s.Context(),
		`UPDATE user_wallets SET address = $1 WHERE user_id = $2`,
		user.Address,
		user.ID.UUID(),
	); err != nil {
		s.Fatalf("flows: update deposit wallet: %v", err)
	}
	if _, err := s.DB().Exec(
		s.Context(),
		`INSERT INTO deposit_cursors (wallet_address, last_signature, cursor_slot, scanned_at)
		VALUES ($1, '', 0, $2) ON CONFLICT (wallet_address) DO NOTHING`,
		user.Address,
		scannedBeforeEveryWallet(),
	); err != nil {
		s.Fatalf("flows: seed deposit cursor: %v", err)
	}
	return user
}

func expectDeposit(user testkit.SeededUser) scenario.Step {
	return func(s *scenario.Scenario) {
		var count int
		if err := s.DB().QueryRow(
			s.Context(),
			`SELECT count(*) FROM deposits WHERE user_id = $1`,
			user.ID.UUID(),
		).Scan(&count); err != nil {
			s.Fatalf("flows: count deposits: %v", err)
		}
		if count != 1 {
			s.Fatalf("flows: deposits = %d, want 1", count)
		}
	}
}
