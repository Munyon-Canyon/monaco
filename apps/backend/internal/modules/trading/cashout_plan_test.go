package trading_test

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	platform "github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func lot(mint string, units, quote uint64) domain.Lot {
	return domain.Lot{
		Mint:   platform.Mint{Address: platform.SolanaAddress(mint), Decimals: 8},
		Symbol: mint, Units: units, QuoteUSDC: quote,
	}
}

func TestPlanCashOutSell(t *testing.T) {
	t.Parallel()
	held := []domain.Lot{lot("TSLAx", 300, 30_000_000), lot("DUSTx", 5, 0), lot("AAPLx", 1_000, 50_000_000)}
	cases := []struct {
		name   string
		needed uint64
		want   []domain.Lot
	}{
		{"one holding covers", 20_000_000, []domain.Lot{lot("AAPLx", 404, 20_200_000)}},
		{"buffer lands exactly on a holding", 49_504_951, []domain.Lot{lot("AAPLx", 1_000, 50_000_000)}},
		{
			"spans two holdings", 60_000_000,
			[]domain.Lot{lot("AAPLx", 1_000, 50_000_000), lot("TSLAx", 106, 10_600_000)},
		},
		{
			"all holdings short sells everything", 100_000_000,
			[]domain.Lot{lot("AAPLx", 1_000, 50_000_000), lot("TSLAx", 300, 30_000_000)},
		},
		{"zero needed", 0, []domain.Lot{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := domain.PlanCashOutSell(held, money.MicrosFromUint64(c.needed))
			if err != nil || !slices.Equal(got, c.want) {
				t.Fatalf("plan = %+v, %v, want %+v", got, err, c.want)
			}
		})
	}
	if held[0].Symbol != "TSLAx" {
		t.Fatalf("planning reordered the caller's holdings: %+v", held)
	}
}

func TestPlanCashOutSell_PartialRoundsUnitsUp(t *testing.T) {
	t.Parallel()
	got, err := domain.PlanCashOutSell([]domain.Lot{lot("AAPLx", 3, 10_000_000)}, money.MicrosFromUint64(990_100))
	if want := []domain.Lot{lot("AAPLx", 1, 3_333_333)}; err != nil || !slices.Equal(got, want) {
		t.Fatalf("plan = %+v, %v, want %+v", got, err, want)
	}
}

func TestPlanCashOutSell_TargetOverflowRejected(t *testing.T) {
	t.Parallel()
	_, err := domain.PlanCashOutSell(nil, money.MicrosFromUint64(math.MaxUint64))
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("err = %v, want %s", err, errs.CodeInvalidInput)
	}
}

func TestPlanCashOutSell_NeverPlansMoreThanHeld(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewPCG(568, 2026))
	for i := range 5_000 {
		held := randomLots(rng)
		needed := randomNeed(rng, held)
		plan, err := domain.PlanCashOutSell(held, money.MicrosFromUint64(needed))
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if msg := planDefect(held, plan, needed); msg != "" {
			t.Fatalf("case %d: %s\nheld %+v\nneeded %d\nplan %+v", i, msg, held, needed, plan)
		}
	}
}

func randomLots(rng *rand.Rand) []domain.Lot {
	out := make([]domain.Lot, rng.IntN(6))
	for i := range out {
		units, quote := rng.Uint64(), rng.Uint64()
		if rng.IntN(2) == 0 {
			units, quote = rng.Uint64N(1_000), rng.Uint64N(1_000_000)
		}
		out[i] = lot(fmt.Sprintf("M%d", i), units, quote)
	}
	return out
}

func randomNeed(rng *rand.Rand, held []domain.Lot) uint64 {
	if len(held) == 0 || rng.IntN(3) == 0 {
		return rng.Uint64N(math.MaxUint64 / 20_000)
	}
	near := held[rng.IntN(len(held))].QuoteUSDC
	near -= min(near, rng.Uint64N(3))
	return near / 101 * 100
}

func planDefect(held, plan []domain.Lot, needed uint64) string {
	unsold := map[platform.SolanaAddress]domain.Lot{}
	for _, h := range held {
		unsold[h.Mint.Address] = h
	}
	partial := len(plan) > 0 && plan[len(plan)-1] != unsold[plan[len(plan)-1].Mint.Address]
	for i, leg := range plan {
		if msg := legDefect(unsold, leg, i == len(plan)-1); msg != "" {
			return fmt.Sprintf("leg %d: %s", i, msg)
		}
		delete(unsold, leg.Mint.Address)
	}
	if partial {
		return ""
	}
	return shortfallDefect(unsold, plan, needed+needed/100)
}

func legDefect(unsold map[platform.SolanaAddress]domain.Lot, leg domain.Lot, last bool) string {
	h, ok := unsold[leg.Mint.Address]
	switch {
	case !ok:
		return "sells a mint the cabal does not hold, or sells one twice"
	case leg.Units == 0 || leg.Units > h.Units:
		return fmt.Sprintf("sells %d units of %d held", leg.Units, h.Units)
	case leg.QuoteUSDC > h.QuoteUSDC:
		return fmt.Sprintf("quotes %d, above the whole holding's %d", leg.QuoteUSDC, h.QuoteUSDC)
	case !last && leg != h:
		return "is partial but is not the last leg"
	}
	return ""
}

func shortfallDefect(unsold map[platform.SolanaAddress]domain.Lot, plan []domain.Lot, target uint64) string {
	var sold uint64
	for _, leg := range plan {
		sold += leg.QuoteUSDC
	}
	for _, h := range unsold {
		if h.Units > 0 && h.QuoteUSDC > 0 && sold < target {
			return fmt.Sprintf("plan stops at %d USDC below the %d target while %s is unsold", sold, target, h.Symbol)
		}
	}
	return ""
}
