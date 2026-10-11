package flows

import (
	"context"
	"math/big"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	RoutedBuyMicros     = 25_000_000
	RoutedBridgeFee     = 7_500
	RoutedRequotedUnits = 306_000_000
	RoutedFilledUnits   = 305_000_000
	RoutedSaleProceeds  = 101_000_000
	RoutedEVMToken      = "0x00000000000000000000000000000000004970aa"
	f30FundedMicros     = 100_000_000
	f30TreasuryEVMHex   = "0x00000000000000000000000000000000004970bb"
)

func (defined) StepsF30FundCabal() FlowStep {
	return FlowStep{Flow: "30", Order: 2, Name: "Fund cabal", Owner: "#4970", Status: StepPending}
}

func (defined) StepsF30RoutedBuy() FlowStep {
	return FlowStep{
		Flow: "30", Order: 3, Name: "Routed buy on EVM", Owner: "#4970", Status: StepReady,
		Script: F30RoutedBuyOnEVM,
	}
}

func RoutedListing() multichain.Listing {
	return multichain.Listing{
		ID:      multichain.ListingIDFrom(uuid.UUID{0: 0x01, 1: 0x9b, 6: 0x70, 8: 0x80, 14: 0x49, 15: 0x70}),
		StockID: f30Stock(), Issuer: "coinbase", Symbol: "AAPLc", Network: chain.NetworkBase,
		TokenAddress: RoutedEVMToken, Decimals: 8, SharesPerToken: big.NewRat(1, 1), Venue: multichain.VenueUniswap,
	}
}

func RoutedTreasuryEVM() multichain.EVMWallet {
	address, _ := chain.ParseEVMAddress(f30TreasuryEVMHex)
	return multichain.EVMWallet{ID: "privy-evm-treasury-f30", Address: address}
}

func f30Stock() multichain.StockID {
	var stock multichain.StockID
	_ = stock.UnmarshalText([]byte("019b0000-0000-7000-8000-000000004971"))
	return stock
}

func f30Comparison(side multichain.Side, input uint64) *multichain.Comparison {
	l := RoutedListing()
	at := clock.Real{}.Now()
	return &multichain.Comparison{
		StockID: l.StockID, Side: side, Winner: l.ID,
		Ranked: []multichain.Quote{{
			ListingID: l.ID, Network: l.Network, Venue: l.Venue, Side: side,
			InputMicros: money.MicrosFromUint64(input), NetMicros: money.MicrosFromUint64(input),
			Costs:    []multichain.Cost{{Kind: multichain.CostBridge, Micros: money.MicrosFromUint64(RoutedBridgeFee)}},
			QuotedAt: at, ExpiresAt: at.Add(time.Minute), RouteID: "route-f30",
		}},
	}
}

func F30RoutedBuyOnEVM(s *scenario.Scenario) {
	f30Buy(s)
	s.Then(
		scenario.ExpectAllEvents(events.TypeTradeConfirmed, 1),
		scenario.ExpectAllEvents(events.TypeTradeStepChanged, 6),
	)
}

func f30Buy(s *scenario.Scenario) ids.CabalID {
	s.Helper()
	cabal := testkit.NewCabal(s, s.DB())
	testkit.NewLedger(s, s.DB()).WithFundedMember(cabal.Creator.ID, cabal.ID, money.MicrosFromUint64(f30FundedMicros))
	l := RoutedListing()
	if _, err := s.DB().Exec(s.Context(), `INSERT INTO asset_listings
		(id, issuer, symbol, network, chain_id, token_address, decimals, shares_per_token, venue, enabled, created_at)
		VALUES ($1, $2, $3, 'base', 8453, $4, $5, 1, 'uniswap', false, now())`,
		l.ID.UUID(), l.Issuer, l.Symbol, l.TokenAddress, int(l.Decimals)); err != nil {
		s.Fatalf("flows: seed the EVM listing: %v", err)
	}
	proposal := f30Pass(s, events.ProposalPassed{
		CabalID: cabal.ID.UUID(), Kind: "buy", USDCMicros: money.MicrosFromUint64(RoutedBuyMicros),
		QuoteOutAmount: RoutedRequotedUnits, PriceComparison: f30Comparison(multichain.SideBuy, RoutedBuyMicros),
	})
	s.When(
		scenario.Eventually("the routed buy is confirmed", f30Swap(proposal, "confirmed")),
		scenario.Eventually("the bought tokens are on the ledger", f30Held(cabal.ID)),
	)
	return cabal.ID
}

func f30Pass(s *scenario.Scenario, ev events.ProposalPassed) uuid.UUID {
	s.Helper()
	aapl := marketfake.AAPLx()
	ev.V, ev.ProposalID, ev.ProposerID = 1, ids.Real{}.NewV7(), ids.Real{}.NewV7()
	ev.Symbol, ev.Mint = aapl.Symbol, aapl.Mint.Address()
	ctx := observability.WithActor(s.Context(), "system:flows")
	err := db.New(s.DB(), ids.Real{}, clock.Real{}).Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return tx.Events.Append(ctx, ev)
	})
	if err != nil {
		s.Fatalf("flows: pass the proposal: %v", err)
	}
	return ev.ProposalID
}

func f30Swap(proposal uuid.UUID, status string) func(*scenario.Scenario) bool {
	return func(s *scenario.Scenario) bool {
		var done bool
		if err := s.DB().QueryRow(s.Context(), `SELECT EXISTS (SELECT 1 FROM swaps
			WHERE source_id = $1 AND status = $2)`, proposal, status).Scan(&done); err != nil {
			s.Fatalf("flows: read the routed swap: %v", err)
		}
		return done
	}
}
