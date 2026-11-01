package app

import (
	"context"
	"math"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chainfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const (
	capTreasury = chain.SolanaAddress("treasury-wallet")
	capUSDC     = chain.SolanaAddress("usdc-mint")
)

type capLedger struct {
	*chainfake.Ledger
	cfg solana.MintConfig
}

func (l capLedger) MintConfig(context.Context, chain.SolanaAddress) (solana.MintConfig, error) {
	return l.cfg, nil
}

func capPorts(cash uint64, cfg solana.MintConfig, bps *int32) (TradePorts, ids.CabalID) {
	id := ids.CabalIDFrom(testkit.NewIDs(1).NewV7())
	rules := cabalport.Rules{MaxTradeBps: bps}
	ledger := chainfake.NewLedger(clock.Real{})
	usdc := chain.Mint{Address: capUSDC, Decimals: 6}
	ledger.SetTokens(capTreasury, usdc, cash)
	seed := fakes.CabalSeed{
		Rules:  rules,
		Wallet: cabalport.TreasuryWallet{CabalID: id, Address: capTreasury},
	}
	seed.View.ID = id
	treasury := fakes.NewTreasury()
	treasury.SetCash(id, capUSDC, cash)
	return TradePorts{
		Cabals: fakes.NewCabal([]fakes.CabalSeed{seed}, nil), Balances: capLedger{Ledger: ledger, cfg: cfg}, USDC: usdc,
		Treasury: treasury,
	}, id
}

func capTrade(kind domain.Kind, micros uint64) domain.Trade {
	return domain.Trade{Kind: kind, Symbol: "AAPLx", USDCMicros: money.MicrosFromUint64(micros)}
}

func TestFundsEnforcesTheCabalsMaxTradeCap(t *testing.T) {
	t.Parallel()
	twenty, full, negative, huge := int32(2000), int32(10_000), int32(-1), int32(20_000)
	feeShort := solana.MintConfig{TransferFeeBps: 100, MaxFee: money.NewBaseUnits(math.MaxUint64, 8)}
	for name, tc := range map[string]struct {
		cash  uint64
		cfg   solana.MintConfig
		bps   *int32
		trade domain.Trade
		want  errs.Code
	}{
		"at the cap": {5_100_000_000, solana.MintConfig{}, &twenty, capTrade(domain.KindBuy, 1_020_000_000), ""},
		"one micro over the cap": {
			5_100_000_000, solana.MintConfig{}, &twenty, capTrade(domain.KindBuy, 1_020_000_001), errs.CodeOverTradeCap,
		},
		"odd cash rounds the cap down, at the cap": {
			5_100_000_001, solana.MintConfig{}, &twenty, capTrade(domain.KindBuy, 1_020_000_000), "",
		},
		"odd cash rounds the cap down, one over": {
			5_100_000_001, solana.MintConfig{}, &twenty, capTrade(domain.KindBuy, 1_020_000_001), errs.CodeOverTradeCap,
		},
		"at the cap but the fee headroom exceeds cash": {
			1_000_000, feeShort, &full,
			capTrade(domain.KindBuy, 1_000_000), errs.CodePotCashShort,
		},
		"negative cap":  {5_100_000_000, solana.MintConfig{}, &negative, capTrade(domain.KindBuy, 1), errs.CodeInternal},
		"cap overflows": {math.MaxUint64, solana.MintConfig{}, &huge, capTrade(domain.KindBuy, 1), errs.CodeInternal},
		"no cap":        {5_100_000_000, solana.MintConfig{}, nil, capTrade(domain.KindBuy, 5_000_000_000), ""},
		"sell over the cap": {
			5_100_000_000, solana.MintConfig{}, &twenty, capTrade(domain.KindSell, 5_000_000_000), "",
		},
	} {
		ports, id := capPorts(tc.cash, tc.cfg, tc.bps)
		pot := money.MicrosFromUint64(math.MaxUint64)
		err := funds(context.Background(), ports, id, marketfake.AAPLx(), tc.trade, pot)
		if tc.want == "" {
			if err != nil {
				t.Errorf("%s: funds = %v, want nil", name, err)
			}
			continue
		}
		if got := errs.CodeOf(err); err == nil || got != tc.want {
			t.Errorf("%s: funds code = %v (%v), want %s", name, got, err, tc.want)
		}
	}
}

func TestOverTradeCapMessageNamesThePercentAndTheCap(t *testing.T) {
	t.Parallel()
	bps := int32(2000)
	ports, id := capPorts(5_100_000_001, solana.MintConfig{}, &bps)
	err := funds(context.Background(), ports, id, marketfake.AAPLx(), capTrade(domain.KindBuy, 1_020_000_001),
		money.MicrosFromUint64(math.MaxUint64))
	const want = "This cabal caps a trade at 20% of its cash ($1,020.00)"
	if got := errs.MessageFor(err, errs.CodeOverTradeCap); got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestFormatPercentAndDollars(t *testing.T) {
	t.Parallel()
	for bps, want := range map[uint64]string{2000: "20%", 2050: "20.5%", 2005: "20.05%", 50: "0.5%"} {
		if got := formatPercent(bps); got != want {
			t.Errorf("formatPercent(%d) = %q, want %q", bps, got, want)
		}
	}
	for micros, want := range map[uint64]string{
		0: "$0.00", 9_999: "$0.00", 10_000: "$0.01", 1_020_000_000: "$1,020.00", 1_234_567_899_999: "$1,234,567.89",
	} {
		if got := formatDollars(micros); got != want {
			t.Errorf("formatDollars(%d) = %q, want %q", micros, got, want)
		}
	}
}

type rulesDown struct{ Cabals }

func (rulesDown) Rules(context.Context, ids.CabalID) (cabalport.Rules, error) {
	return cabalport.Rules{}, errs.New(errs.CodeUpstreamUnavailable, "test")
}

func TestFundsReturnsTheRulesError(t *testing.T) {
	t.Parallel()
	ports, id := capPorts(5_100_000_000, solana.MintConfig{}, nil)
	ports.Cabals = rulesDown{ports.Cabals}
	err := funds(context.Background(), ports, id, marketfake.AAPLx(), capTrade(domain.KindBuy, 1),
		money.MicrosFromUint64(math.MaxUint64))
	if got := errs.CodeOf(err); got != errs.CodeUpstreamUnavailable {
		t.Errorf("funds code = %v (%v), want upstream_unavailable", got, err)
	}
}
