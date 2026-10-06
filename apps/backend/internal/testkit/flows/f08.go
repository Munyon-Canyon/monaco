package flows

import (
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	reconcileTreasury  = "7Moh9vxpdx8awiKHGpWLFze6sXVeZgBphdP2EopQ78UG"
	reconcileWallet    = "wallet-0199c0de-0008-7000-8000-000000000008"
	reconcilePoller    = "funding.treasury-reconcile"
	reconcileUSDCSig   = "4cu1Tu8V1ZuQdTNo5qjiDtDfaFsoaKmRW6dXW8pVSsHMhPcV1wsCDzDLrCzTehfLfSTQSva26SS7uUUVB8nswqp9"
	reconcileDustSig   = "4ABXAAQc1LWqaGi5dMVZZEf12hykGXmHWU6zR3bkUruM6pnNaV78QHVRp2M53ojHEbvRpCWE2cKMZoQEugfoNi4h"
	reconcileOtherSig  = "sfLpFuMu7roifBc8qhCc2tnFUKtyscedzXgdRgNnm22w6YvDmYogC9UkjgYWZ6Md5A6a215gfE5PGcb6CAPrA1k"
	reconcileClosedSig = "r3oLbEs3YRBcwXJUqrYiQiAU4xahpmcfwXZdMk77oxeyn7Y53tTxZ8yvySU1DNvMaaYqrodaEUoSshRQk67uYau"
	reconcileSender    = "6CBBbEVgz7TsMc9yuU3tekHwnjE6BiSwFb4afWDrc8gd"
	reconcileStray     = 25_000_000
	reconcileStake     = 2_000_000
)

func (defined) WorkerEnvF08() []string {
	return []string{
		"FUNDING_TREASURY_RECONCILE_INTERVAL=1s", "FUNDING_BOUNCE_SWEEP_INTERVAL=1s", "FUNDING_BOUNCE_SWEEP_AGE=2s",
		"RELAYER_PRIVATE_KEY=" + CashOutRelayerKey(),
	}
}

func seedStrayTransfers(s *scenario.Scenario, cabal ids.CabalID) []scenario.Step {
	if _, err := s.DB().Exec(s.Context(),
		`UPDATE treasury_wallets SET address = $1, privy_wallet_id = 'retired-' || cabal_id WHERE address = $2`,
		string(freshAddress(s)), reconcileTreasury); err != nil {
		s.Fatalf("flows: free the fixture treasury from an earlier run: %v", err)
	}
	if _, err := s.DB().Exec(s.Context(),
		`UPDATE treasury_wallets SET address = $1, privy_wallet_id = $2 WHERE cabal_id = $3`,
		reconcileTreasury, reconcileWallet, cabal.UUID()); err != nil {
		s.Fatalf("flows: point the treasury at the fixture wallet: %v", err)
	}
	if _, err := s.DB().Exec(s.Context(), `WITH retired AS (
			UPDATE external_deposits
			SET signature = 'retired-' || id, bounce_signature = 'retired-bounce-' || id,
				status = CASE WHEN status IN ('detected', 'bouncing', 'bounce_failed') THEN 'held' ELSE status END,
				resolved_at = coalesce(resolved_at, now())
			WHERE signature = ANY($1)
			RETURNING id)
		UPDATE cabal_pauses SET resolved_at = now()
		WHERE resolved_at IS NULL AND external_deposit_id IN (SELECT id FROM retired)`,
		[]string{reconcileUSDCSig, reconcileDustSig, reconcileOtherSig, reconcileClosedSig}); err != nil {
		s.Fatalf("flows: hold the fixture transfers an earlier run recorded: %v", err)
	}
	return []scenario.Step{
		signaturesListed(""),
		scenario.FakeUpstream(fakes.Step{Route: "/rpc/getTransaction", Action: fakes.ActionSucceed, Reset: true}),
		scenario.FakeUpstream(fakes.Step{Route: "/rpc/getSignatureStatuses", Action: fakes.ActionSucceed, Reset: true}),
	}
}

func externalDepositIs(sig string, statuses ...string) scenario.Step {
	return scenario.Eventually("external deposit "+sig+" in "+statuses[0], func(s *scenario.Scenario) bool {
		var status string
		if err := s.DB().QueryRow(s.Context(), `SELECT status FROM external_deposits WHERE signature = $1`,
			sig).Scan(&status); err != nil {
			return false
		}
		return slices.Contains(statuses, status)
	})
}

func bounceLands(landed bool) scenario.Step {
	step := fakes.Step{Route: "/rpc/getSignatureStatuses", Action: fakes.ActionSucceed, Reset: true}
	if !landed {
		step.Fixture, step.Times = "/rpc/getSignatureStatuses/not-found", 1000
	}
	return scenario.FakeUpstream(step)
}

func pausedFor(reason string) scenario.Step {
	return scenario.ExpectField("pause", func(s *scenario.Scenario, raw json.RawMessage) {
		var pause struct {
			Reasons []string `json:"reasons"`
		}
		if err := json.Unmarshal(raw, &pause); err != nil || !slices.Contains(pause.Reasons, reason) {
			s.Fatalf("flows: preview pause %s, want reasons with %s", raw, reason)
		}
	})
}

func cabalEvents(cabal ids.CabalID, want map[events.Type]int) scenario.Step {
	return func(s *scenario.Scenario) {
		for typ, n := range want {
			var got int
			if err := s.DB().QueryRow(s.Context(),
				`SELECT count(*) FROM events WHERE type = $1 AND aggregate_id = $2`, string(typ), cabal.UUID(),
			).Scan(&got); err != nil {
				s.Fatalf("flows: count %s events: %v", typ, err)
			}
			if got != n {
				s.Fatalf("flows: %d %s events for cabal %s, want %d", got, typ, cabal, n)
			}
		}
	}
}

func sentBackOnce(cabal ids.CabalID) []scenario.Step {
	return []scenario.Step{
		scenario.EventuallyCabalEvent(cabal, events.TypeCabalExternalDepositBounced),
		scenario.EventuallyCabalEvent(cabal, events.TypeCabalResumed),
		scenario.ExpectEventPayload(events.TypeCabalExternalDepositBounced, map[string]any{
			"cabal_id": cabal.String(), "recipient": reconcileSender, "mint": testkit.USDCMint,
			"amount": "25000000",
		}),
		cabalEvents(cabal, map[events.Type]int{
			events.TypeCabalExternalDepositDetected: 1, events.TypeCabalExternalDepositBounced: 1,
			events.TypeCabalPaused: 1, events.TypeCabalResumed: 1,
		}),
		func(s *scenario.Scenario) {
			var sends int
			if err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM external_deposits
				WHERE cabal_id = $1 AND status = 'returned' AND bounce_signature IS NOT NULL AND amount = $2`,
				cabal.UUID(), reconcileStray).Scan(&sends); err != nil || sends != 1 {
				s.Fatalf("flows: %d returned bounces of %d (%v), want 1", sends, reconcileStray, err)
			}
		},
	}
}

func F08DetectExternalDepositOK(s *scenario.Scenario) {
	f := seedFund(s)
	testkit.NewLedger(seedT{s}, s.DB()).
		WithFundedMember(f.member.ID, f.cabal.ID, money.MicrosFromUint64(reconcileStake))
	cabal, preview := f.cabal.ID, "/v1/cabals/"+f.cabal.ID.String()+"/cashouts/preview"
	s.Given(append(seedStrayTransfers(s, cabal), bounceLands(false), scenario.AsSeededUser("member", f.member.ID))...).
		When(append([]scenario.Step{
			scenario.AwaitTick(reconcilePoller),
			externalDepositIs(reconcileUSDCSig, "bouncing"),
			scenario.Post(f.path(), fundBody),
			scenario.ExpectProblem(errs.CodeCabalPaused),
			scenario.Post("/v1/cabals/"+cabal.String()+"/cashouts", cashOutOf("1000000")),
			scenario.ExpectProblem(errs.CodeCabalPaused),
			scenario.Get(preview),
			scenario.ExpectStatus(http.StatusOK),
			pausedFor("external_deposit"),
			bounceLands(true),
			externalDepositIs(reconcileUSDCSig, "returned"),
		}, sentBackOnce(cabal)...)...).
		Then(
			scenario.Get(preview),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("pause", nil),
			scenario.Post(f.path(), fundBody),
			scenario.ExpectStatus(http.StatusAccepted),
			scenario.ExpectJSON("status", "submitted"),
		)
}

func F08DetectExternalDepositDust(s *scenario.Scenario) {
	s.Given(seedStrayTransfers(s, testkit.NewCabal(seedT{s}, s.DB()).ID)...).When(
		scenario.AwaitTick(reconcilePoller),
		externalDepositIs(reconcileDustSig, "ignored_dust"),
	).Then()
}

func F08DetectExternalDepositUnknownAsset(s *scenario.Scenario) {
	s.Given(seedStrayTransfers(s, testkit.NewCabal(seedT{s}, s.DB()).ID)...).When(
		scenario.AwaitTick(reconcilePoller),
		externalDepositIs(reconcileOtherSig, "ignored_unknown"),
	).Then()
}

func signaturesListed(fixture string) scenario.Step {
	step := fakes.Step{Route: "/rpc/getSignaturesForAddress", Action: fakes.ActionSucceed, Reset: true}
	if fixture != "" {
		step.Fixture, step.Times = "/rpc/getSignaturesForAddress/"+fixture, 1000
	}
	return scenario.FakeUpstream(step)
}

func F08DetectExternalDepositBounceFailed(s *scenario.Scenario) {
	cabal := testkit.NewCabal(seedT{s}, s.DB()).ID
	closed := map[string]string{"reason": "recipient_token_account_closed"}
	s.Given(append(seedStrayTransfers(s, cabal), signaturesListed("closed-sender"))...).
		When(
			scenario.AwaitTick(reconcilePoller),
			externalDepositIs(reconcileClosedSig, "bounce_failed"),
			scenario.EventuallyLog(observability.FundingBounceFailed, closed),
		).
		Then(
			cabalEvents(cabal, map[events.Type]int{
				events.TypeCabalPaused: 1, events.TypeCabalResumed: 0, events.TypeCabalExternalDepositBounced: 0,
			}),
			signaturesListed(""),
		)
}

func strayCrashes(s *scenario.Scenario, point faultpoint.Name) {
	cabal := testkit.NewCabal(seedT{s}, s.DB()).ID
	s.Given(seedStrayTransfers(s, cabal)...).
		When(scenario.PublishCrashingAt(point), externalDepositIs(reconcileUSDCSig, "returned")).
		Then(sentBackOnce(cabal)...)
}

func F08DetectExternalDepositCrashBeforeCommit(s *scenario.Scenario) {
	strayCrashes(s, faultpoint.BeforeCommit)
}

func F08DetectExternalDepositCrashAfterSign(s *scenario.Scenario) {
	strayCrashes(s, faultpoint.AfterSign)
}

func F08DetectExternalDepositCrashAfterBroadcast(s *scenario.Scenario) {
	strayCrashes(s, faultpoint.AfterBroadcast)
}

func freshAddress(s *scenario.Scenario) chain.SolanaAddress {
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		s.Fatalf("flows: generate an address: %v", err)
	}
	return chain.AddressOf(pub)
}
