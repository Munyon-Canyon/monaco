package flows

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
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
	s.When(
		scenario.Eventually("the sale proceeds are back on Solana", f30Returned(proposal)),
		scenario.Eventually("the EVM position is sold", f30Sold(cabal)),
		scenario.Eventually("the landed USDC is on the cabal's Solana ledger", f30Landed(cabal, proposal)),
	).Then(
		scenario.ExpectAllEvents(events.TypeTradeConfirmed, 2),
		scenario.ExpectAllEvents(events.TypeTradeStepChanged, 12),
	)
}

func (defined) StepsF30GovernedSell() FlowStep {
	return FlowStep{
		Flow: "30", Order: 5, Name: "Governed routed sell", Owner: "#5851", Status: StepReady, Script: F30GovernedSell,
	}
}

const f30PartialSale = 100_000_000

func F30GovernedSell(s *scenario.Scenario) {
	cabal := f30Bought(s)
	left := uint64(RoutedFilledUnits - f30PartialSale)
	s.Given(scenario.AsSeededUser("member", cabal.Creator.ID)).
		When(f30GovernedSale(cabal.ID, f30PartialSale, left)...).
		When(f30GovernedSale(cabal.ID, left, 0)...).
		Then(
			scenario.ExpectAllEvents(events.TypeTradeConfirmed, 3),
			scenario.ExpectEventPayload(events.TypeProposalPassed, map[string]any{
				"kind": "sell", "network": "base", "token_amount": strconv.Itoa(f30PartialSale),
			}),
		)
}

func f30GovernedSale(cabal ids.CabalID, units, left uint64) []scenario.Step {
	body := fmt.Sprintf(`{"kind":"sell","symbol":"AAPLx","token_amount":%d,"network":"base"}`, units)
	proposal := func(s *scenario.Scenario) uuid.UUID { return uuid.MustParse(s.Recall("proposal")) }
	return []scenario.Step{
		scenario.Post("/v1/cabals/"+cabal.String()+"/proposals", body),
		scenario.ExpectStatus(http.StatusCreated),
		scenario.ExpectJSON("network", "base"),
		scenario.ExpectJSON("token_amount", units),
		scenario.Remember("id", "proposal"),
		scenario.Post("/v1/proposals/{proposal}/votes", `{"choice":"yes"}`),
		scenario.ExpectStatus(http.StatusOK),
		scenario.ExpectJSON("status", "passed"),
		scenario.Eventually("the sale proceeds are back on Solana", func(s *scenario.Scenario) bool {
			return f30Returned(proposal(s))(s)
		}),
		scenario.Eventually("the EVM position keeps exactly what was not sold", f30Left(cabal, left)),
		scenario.Eventually("the landed USDC is on the cabal's Solana ledger", f30LandedTotal(cabal)),
	}
}

func f30Left(cabal ids.CabalID, units uint64) func(*scenario.Scenario) bool {
	return func(s *scenario.Scenario) bool {
		var held bool
		if err := s.DB().QueryRow(s.Context(), `SELECT coalesce(
			(SELECT units = $3 FROM cabal_positions WHERE cabal_id = $1 AND asset = $2), $3 = 0)`,
			cabal.UUID(), "base:"+RoutedEVMToken, units).Scan(&held); err != nil {
			s.Fatalf("flows: read the position left: %v", err)
		}
		return held
	}
}

func f30LandedTotal(cabal ids.CabalID) func(*scenario.Scenario) bool {
	return func(s *scenario.Scenario) bool {
		var landed bool
		if err := s.DB().QueryRow(s.Context(), `SELECT EXISTS (
			SELECT 1 FROM cabal_positions AS p
			WHERE p.cabal_id = $1 AND p.asset = $2 AND p.units = $3::numeric - $4::numeric + (
				SELECT coalesce(sum((e.payload->>'amount_micros')::numeric - (e.payload->>'fee_micros')::numeric), 0)
				FROM events e JOIN swaps w ON w.id = e.aggregate_id
				WHERE w.cabal_id = $1 AND w.action = 'sell' AND e.type = 'trade.step_changed'
				AND e.payload->>'step' = 'bridge_back' AND e.payload->>'status' = 'succeeded'))`,
			cabal.UUID(), string(testkit.USDCMint), f30FundedMicros, RoutedBuyMicros,
		).Scan(&landed); err != nil {
			s.Fatalf("flows: read the landed total: %v", err)
		}
		return landed
	}
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

func f30Sold(cabal ids.CabalID) func(*scenario.Scenario) bool {
	return func(s *scenario.Scenario) bool {
		var held bool
		if err := s.DB().QueryRow(s.Context(), `SELECT EXISTS (SELECT 1 FROM cabal_positions
			WHERE cabal_id = $1 AND asset = $2 AND units > 0)`,
			cabal.UUID(), "base:"+RoutedEVMToken).Scan(&held); err != nil {
			s.Fatalf("flows: read the sold position: %v", err)
		}
		return !held
	}
}

func f30Landed(cabal ids.CabalID, proposal uuid.UUID) func(*scenario.Scenario) bool {
	return func(s *scenario.Scenario) bool {
		var landed bool
		if err := s.DB().QueryRow(s.Context(), `SELECT EXISTS (
			SELECT 1 FROM cabal_positions AS p
			JOIN swaps AS w ON w.source_id = $2
			JOIN events AS e ON e.aggregate_id = w.id AND e.type = 'trade.step_changed'
				AND e.payload->>'step' = 'bridge_back' AND e.payload->>'status' = 'succeeded'
			WHERE p.cabal_id = $1 AND p.asset = $3 AND p.units = $4::numeric - $5::numeric
				+ (e.payload->>'amount_micros')::numeric - (e.payload->>'fee_micros')::numeric)`,
			cabal.UUID(), proposal, string(testkit.USDCMint), f30FundedMicros, RoutedBuyMicros,
		).Scan(&landed); err != nil {
			s.Fatalf("flows: read the landed proceeds: %v", err)
		}
		return landed
	}
}
