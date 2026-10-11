package flows

import (
	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func (defined) StepsF30Sell() FlowStep {
	return FlowStep{
		Flow: "30", Order: 4, Name: "Routed sell", Owner: "#4970", Status: StepReady, Script: F30RoutedSell,
	}
}

func F30RoutedSell(s *scenario.Scenario) {
	cabal := f30Buy(s)
	proposal := f30Pass(s, events.ProposalPassed{
		CabalID: cabal.UUID(), Kind: "sell", TokenAmount: RoutedFilledUnits, QuoteOutAmount: RoutedSaleProceeds,
		PriceComparison: f30Comparison(multichain.SideSell, RoutedSaleProceeds),
	})
	s.When(scenario.Eventually("the sale proceeds are back on Solana", f30Returned(proposal))).Then(
		scenario.ExpectAllEvents(events.TypeTradeConfirmed, 2),
		scenario.ExpectAllEvents(events.TypeTradeStepChanged, 12),
	)
}

func f30Held(cabal ids.CabalID) func(*scenario.Scenario) bool {
	return func(s *scenario.Scenario) bool {
		var held bool
		if err := s.DB().QueryRow(s.Context(), `SELECT EXISTS (SELECT 1 FROM cabal_positions
			WHERE cabal_id = $1 AND asset = $2 AND units = $3)`,
			cabal.UUID(), "base:"+RoutedEVMToken, RoutedFilledUnits).Scan(&held); err != nil {
			s.Fatalf("flows: read the bought position: %v", err)
		}
		return held
	}
}

func f30Returned(proposal uuid.UUID) func(*scenario.Scenario) bool {
	return func(s *scenario.Scenario) bool {
		var done bool
		if err := s.DB().QueryRow(s.Context(), `SELECT EXISTS (SELECT 1 FROM trade_steps t
			JOIN swaps w ON w.id = t.trade_id
			WHERE w.source_id = $1 AND w.status = 'confirmed' AND t.step = 'bridge_back'
			AND t.status = 'succeeded')`, proposal).Scan(&done); err != nil {
			s.Fatalf("flows: read the routed sale: %v", err)
		}
		return done
	}
}
