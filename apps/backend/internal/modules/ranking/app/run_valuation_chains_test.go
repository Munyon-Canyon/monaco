package app

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	chainsUSDC = chain.SolanaAddress("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	baseAAPL   = "base:0xb200000000000000000000000000000000000a01"
)

type bridgeStage struct {
	solana, evmCash, inTransit uint64
}

func listingPrice(observed time.Time) market.ListingPrice {
	p := market.ListingPrice{Price: &market.Price{Micros: money.MicrosFromUint64(30_000_000), ObservedAt: observed}}
	p.Decimals, p.SharesPerToken = 18, big.NewRat(1, 1)
	return p
}

func chainPorts(id ids.CabalID, stage bridgeStage, prices map[market.ListingKey]market.ListingPrice) *countingPorts {
	half, _ := new(big.Int).SetString("500000000000000000", 10)
	shares := money.SharesUnitsFromUint64(100)
	return &countingPorts{
		cabals: []cabal.View{{ID: id}},
		positionRows: []treasury.CabalPositions{{
			CabalID: id, TotalShares: shares,
			Holdings:  []treasury.Position{{Mint: chainsUSDC, Units: money.NewBaseUnits(stage.solana, 6)}},
			EVMCash:   money.MicrosFromUint64(stage.evmCash),
			InTransit: money.MicrosFromUint64(stage.inTransit),
			Listings:  []treasury.ListingPosition{{Key: baseAAPL, Units: half}},
		}},
		stakeRows: []treasury.MemberStake{
			{CabalID: id, UserID: ids.UserIDFrom(ids.Real{}.NewV7()), ShareUnits: shares},
		},
		listingRows: prices,
	}
}

func runChains(t *testing.T, f *countingPorts) Valuation {
	t.Helper()
	got, err := NewRunValuation(
		Ports{Market: f, Treasury: f, Funding: f, Cabals: f, Users: f, Previous: f, Snapshots: f}, chainsUSDC,
	).Run(t.Context(), valuationTime())
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestRunValuation_valuesEveryChainAndStaysFlatAcrossABridge(t *testing.T) {
	t.Parallel()
	prices := map[market.ListingKey]market.ListingPrice{baseAAPL: listingPrice(valuationTime().Add(-time.Minute))}
	for name, stage := range map[string]bridgeStage{
		"before the bridge": {solana: 92_000_000},
		"mid-bridge":        {solana: 80_000_000, inTransit: 12_000_000},
		"landed on Base":    {solana: 80_000_000, evmCash: 12_000_000},
	} {
		id := ids.CabalIDFrom(ids.Real{}.NewV7())
		got := runChains(t, chainPorts(id, stage, prices))
		if len(got.Cabals) != 1 || got.Cabals[0].Value.Uint64() != 107_000_000 {
			t.Fatalf("%s: cabals = %+v, want NAV 107 USDC (92 cash + 0.5 AAPL at 30)", name, got.Cabals)
		}
	}
}

func TestRunValuation_flagsACabalWhoseEVMListingHasNoFreshPrice(t *testing.T) {
	t.Parallel()
	stale := valuationTime().Add(-multichain.ListingPriceMaxAge - time.Second)
	unpriced := listingPrice(valuationTime())
	unpriced.Price = nil
	for name, prices := range map[string]map[market.ListingKey]market.ListingPrice{
		"absent":   {},
		"no price": {baseAAPL: unpriced},
		"stale":    {baseAAPL: listingPrice(stale)},
	} {
		id := ids.CabalIDFrom(ids.Real{}.NewV7())
		got := runChains(t, chainPorts(id, bridgeStage{solana: 92_000_000}, prices))
		if len(got.Cabals) != 0 || len(got.Flagged) != 1 || got.Flagged[0].Flags[0] != domain.FlagUnpricedAssets {
			t.Fatalf("%s: valuation = %+v, want the cabal flagged unpriced, never valued", name, got)
		}
	}
}

func TestRunValuation_failsWhenTheListingReadFails(t *testing.T) {
	t.Parallel()
	f := chainPorts(ids.CabalIDFrom(ids.Real{}.NewV7()), bridgeStage{solana: 1}, nil)
	f.listingsErr = errs.New(errs.CodeDBUnavailable, "test")
	_, err := NewRunValuation(
		Ports{Market: f, Treasury: f, Funding: f, Cabals: f, Users: f, Previous: f, Snapshots: f}, chainsUSDC,
	).Run(t.Context(), valuationTime())
	if errs.CodeOf(err) != errs.CodeDBUnavailable || f.listingCalls != 1 {
		t.Fatalf("Run = %v after %d listing reads, want db_unavailable after one", err, f.listingCalls)
	}
}

func TestOffSolana_marksTheCabalUnpricedAndSaysWhy(t *testing.T) {
	t.Parallel()
	in := valuationInput{cash: money.MicrosFromUint64(^uint64(0)), reserved: money.MicrosFromUint64(3)}
	got, miss := offSolana(in, treasury.CabalPositions{EVMCash: money.MicrosFromUint64(1)}, nil, valuationTime())
	if !got.unpriced || got.reserved.Uint64() != 3 || miss == nil || miss.reason != "overflow" {
		t.Fatalf("offSolana past uint64 = %+v, %+v, want unpriced for overflow with the reservation kept", got, miss)
	}
	unvalued := listingPrice(valuationTime())
	unvalued.SharesPerToken = big.NewRat(1<<62, 1)
	huge, _ := new(big.Int).SetString("1000000000000000000000000", 10)
	position := treasury.CabalPositions{Listings: []treasury.ListingPosition{{Key: baseAAPL, Units: huge}}}
	got, miss = offSolana(valuationInput{}, position, map[market.ListingKey]market.ListingPrice{baseAAPL: unvalued},
		valuationTime())
	if !got.unpriced || miss == nil || miss.reason != "unvalued" || miss.key != baseAAPL {
		t.Fatalf("offSolana with an unvaluable listing = %+v, %+v, want unpriced for %s", got, miss, baseAAPL)
	}
	if got, miss := offSolana(valuationInput{unpriced: true}, position, nil, valuationTime()); !got.unpriced ||
		miss != nil {
		t.Fatalf("offSolana on an unpriced input = %+v, %+v, want it passed through unlogged", got, miss)
	}
}

func TestUnpricedLog_logsEachCabalListingAndReasonOnceEveryTenMinutes(t *testing.T) {
	t.Parallel()
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
	l := &unpricedLog{last: map[string]time.Time{}}
	cabal := ids.CabalIDFrom(ids.Real{}.NewV7())
	stale := &unpricedListing{key: baseAAPL, reason: "stale"}
	for _, after := range []time.Duration{0, 9 * time.Minute, 10 * time.Minute} {
		l.log(ctx, cabal, stale, valuationTime().Add(after))
	}
	l.log(ctx, cabal, nil, valuationTime())
	var lines []map[string]any
	for line := range strings.Lines(string(logs.Bytes())) {
		var got map[string]any
		if json.Unmarshal([]byte(line), &got) == nil && got["msg"] == "ranking.cabal.unpriced_listing" {
			lines = append(lines, got)
		}
	}
	if len(lines) != 2 || lines[0]["cabal"] != cabal.String() || lines[0]["listing"] != baseAAPL ||
		lines[0]["reason"] != "stale" || lines[0]["level"] != "WARN" {
		t.Fatalf("unpriced lines = %v, want a stale warning at 0 and 10 minutes, none at 9", lines)
	}
}
