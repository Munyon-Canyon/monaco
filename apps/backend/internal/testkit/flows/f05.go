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
	depositPoller = "funding.deposit_watch"

	depositCreditWithin = 10 * time.Second
	depositCreditPoll   = 50 * time.Millisecond
)

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
	).Then(scenario.EventuallyCapturedBy(events.TypeDepositCredited, "deposit_credited", "user_id", user.ID.String()))
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
	depositCrashes(s, faultpoint.BeforeCommit)
}

func F05CreditDepositCrashAfterCandidate(s *scenario.Scenario) {
	depositCrashes(s, faultpoint.AfterCandidate)
}

func depositCrashes(s *scenario.Scenario, point faultpoint.Name) {
	user := seedDepositWallet(s)
	s.Given(oneInboundTransfer(user)...).When(
		scenario.TickCrashingAt(depositPoller, point),
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

func F05CreditDepositCrashFirstSightBeforeCommit(s *scenario.Scenario) {
	firstSightCrashes(s, faultpoint.FirstSightBeforeCommit)
}

func F05CreditDepositCrashFirstSightAfterCommit(s *scenario.Scenario) {
	firstSightCrashes(s, faultpoint.FirstSightAfterCommit)
}

const fixtureTransferBlockTime = 1790000000

func firstSightCrashes(s *scenario.Scenario, point faultpoint.Name) {
	user := seedDepositUser(s)
	if _, err := s.DB().Exec(
		s.Context(),
		`UPDATE user_wallets SET created_at = to_timestamp($1) WHERE user_id = $2`,
		fixtureTransferBlockTime-100,
		user.ID.UUID(),
	); err != nil {
		s.Fatalf("flows: backdate deposit wallet: %v", err)
	}
	s.Given(oneInboundTransfer(user)...).When(
		scenario.TickCrashingAt(depositPoller, point),
		scenario.AwaitTick(depositPoller),
		scenario.AwaitTick(depositPoller),
		expectDeposit(user),
	).Then(scenario.ExpectAllEvents(events.TypeDepositCredited, 1))
}

func seedDepositWallet(s *scenario.Scenario) testkit.SeededUser {
	user := seedDepositUser(s)
	ata, err := chain.AssociatedTokenAccount(user.Address, testkit.USDCMint, chain.SPLProgram)
	if err != nil {
		s.Fatalf("flows: derive deposit account: %v", err)
	}
	if _, err := s.DB().Exec(
		s.Context(),
		`INSERT INTO deposit_watch_wallets (wallet_address, user_id, first_seen_slot, first_seen_at, discovery_due_at,
			reconcile_due_at)
		VALUES ($1, $2, 0, now(), now() + interval '1 day', now() + interval '1 day')
		ON CONFLICT (wallet_address) DO UPDATE
		SET user_id = EXCLUDED.user_id, first_seen_slot = 0, discovery_due_at = EXCLUDED.discovery_due_at,
			reconcile_due_at = EXCLUDED.reconcile_due_at`,
		user.Address,
		user.ID.UUID(),
	); err != nil {
		s.Fatalf("flows: seed deposit watch wallet: %v", err)
	}
	if _, err := s.DB().Exec(
		s.Context(),
		`INSERT INTO deposit_watch_accounts (
			token_account, wallet_address, canonical, state, dirty_gen, recovery_due_at)
		VALUES ($1, $2, true, 'open', 1, now() + interval '1 day')
		ON CONFLICT (token_account) DO UPDATE SET wallet_address = EXCLUDED.wallet_address, canonical = true, state = 'open', high_signature = NULL, high_slot = 0,
			page_before = NULL, page_top_signature = NULL, page_top_slot = NULL, recovery_before = NULL,
			recovery_due_at = EXCLUDED.recovery_due_at, dirty_gen = deposit_watch_accounts.clean_gen + 1`,
		ata,
		user.Address,
	); err != nil {
		s.Fatalf("flows: seed deposit watch account: %v", err)
	}
	return user
}

func seedDepositUser(s *scenario.Scenario) testkit.SeededUser {
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
	return user
}

func expectDeposit(user testkit.SeededUser) scenario.Step {
	return func(s *scenario.Scenario) {
		deadline := time.Now().Add(depositCreditWithin)
		var count int
		for {
			if err := s.DB().QueryRow(
				s.Context(),
				`SELECT count(*) FROM deposits WHERE user_id = $1`,
				user.ID.UUID(),
			).Scan(&count); err != nil {
				s.Fatalf("flows: count deposits: %v", err)
			}
			if count != 0 || time.Now().After(deadline) {
				break
			}
			time.Sleep(depositCreditPoll)
		}
		if count != 1 {
			s.Fatalf("flows: deposits = %d, want 1", count)
		}
	}
}
