package flows

import (
	"crypto/ed25519"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	reconcileTreasury = "Aiegw7e8Uy2LbuR5ZULUzorxR8wHsKTMXbBQtY59BbYV"
	reconcilePoller   = "funding.treasury-reconcile"
	reconcileUSDCSig  = "4cu1Tu8V1ZuQdTNo5qjiDtDfaFsoaKmRW6dXW8pVSsHMhPcV1wsCDzDLrCzTehfLfSTQSva26SS7uUUVB8nswqp9"
	reconcileDustSig  = "4ABXAAQc1LWqaGi5dMVZZEf12hykGXmHWU6zR3bkUruM6pnNaV78QHVRp2M53ojHEbvRpCWE2cKMZoQEugfoNi4h"
	reconcileOtherSig = "sfLpFuMu7roifBc8qhCc2tnFUKtyscedzXgdRgNnm22w6YvDmYogC9UkjgYWZ6Md5A6a215gfE5PGcb6CAPrA1k"
)

func (defined) WorkerEnvF08() []string { return []string{"FUNDING_TREASURY_RECONCILE_INTERVAL=1s"} }

func seedStrayTransfers(s *scenario.Scenario) []scenario.Step {
	cabal := testkit.NewCabal(seedT{s}, s.DB())
	if _, err := s.DB().Exec(s.Context(), `UPDATE treasury_wallets SET address = $1 WHERE address = $2`,
		string(freshAddress(s)), reconcileTreasury); err != nil {
		s.Fatalf("flows: free the fixture address from an earlier run: %v", err)
	}
	if _, err := s.DB().Exec(s.Context(), `UPDATE treasury_wallets SET address = $1 WHERE cabal_id = $2`,
		reconcileTreasury, cabal.ID.UUID()); err != nil {
		s.Fatalf("flows: point the treasury at the fixture address: %v", err)
	}
	return []scenario.Step{
		scenario.FakeUpstream(fakes.Step{
			Route: "/rpc/getSignaturesForAddress", Action: fakes.ActionSucceed,
			Fixture: "/rpc/getSignaturesForAddress/" + reconcileTreasury, Times: 100, Reset: true,
		}),
		scenario.FakeUpstream(fakes.Step{
			Route: "/rpc/getTransaction", Action: fakes.ActionSucceed, Times: 100, Reset: true,
		}),
	}
}

func externalDepositIs(sig string, statuses ...string) scenario.Step {
	return scenario.Eventually("external deposit "+sig+" in "+statuses[0], func(s *scenario.Scenario) bool {
		var status string
		if err := s.DB().QueryRow(s.Context(), `SELECT status FROM external_deposits WHERE signature = $1`,
			sig).Scan(&status); err != nil {
			return false
		}
		for _, want := range statuses {
			if status == want {
				return true
			}
		}
		return false
	})
}

func F08DetectExternalDepositOK(s *scenario.Scenario) {
	s.Given(seedStrayTransfers(s)...).When(
		scenario.AwaitTick(reconcilePoller),
		externalDepositIs(reconcileUSDCSig, "detected", "bouncing", "returned", "bounce_failed"),
		scenario.EventuallyEvent(events.TypeCabalExternalDepositDetected),
		scenario.EventuallyEvent(events.TypeCabalPaused),
	).Then()
}

func F08DetectExternalDepositDust(s *scenario.Scenario) {
	s.Given(seedStrayTransfers(s)...).When(
		scenario.AwaitTick(reconcilePoller),
		externalDepositIs(reconcileDustSig, "ignored_dust"),
	).Then()
}

func F08DetectExternalDepositUnknownAsset(s *scenario.Scenario) {
	s.Given(seedStrayTransfers(s)...).When(
		scenario.AwaitTick(reconcilePoller),
		externalDepositIs(reconcileOtherSig, "ignored_unknown"),
	).Then()
}

func F08DetectExternalDepositBounceFailed(s *scenario.Scenario) {
	s.Given(seedStrayTransfers(s)...).When(
		scenario.AwaitTick(reconcilePoller),
		externalDepositIs(reconcileUSDCSig, "bounce_failed"),
		scenario.EventuallyLog(observability.FundingBounceFailed, map[string]string{"reason": "recipient_token_account_closed"}),
	).
		Then()
}

func freshAddress(s *scenario.Scenario) chain.SolanaAddress {
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		s.Fatalf("flows: generate an address: %v", err)
	}
	return chain.AddressOf(pub)
}
