package domain_test

import (
	"math"
	"math/big"
	"reflect"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestProperty_ConservationHoldsForAnyShareSplit(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		nav := usd(rapid.Uint64().Draw(t, "nav"))
		split := rapid.SliceOfN(rapid.Uint64Range(1, 1<<40), 1, 50).Draw(t, "shares")
		var total uint64
		for _, s := range split {
			total += s
		}
		equities := make([]money.Micros, len(split))
		for i, s := range split {
			e, err := domain.MemberEquity(shares(s), shares(total), nav)
			if err != nil {
				t.Fatalf("MemberEquity(%d of %d, %v) = %v", s, total, nav, err)
			}
			equities[i] = e
		}
		if err := domain.CheckConservation(nav, equities); err != nil {
			t.Fatalf("CheckConservation(%v, %v) = %v", nav, equities, err)
		}
	})
}

func TestProperty_CabalNAVNeverExceedsTheExactValue(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		in, exact := drawNAVInput(t)
		got, err := domain.CabalNAV(in)
		if err != nil {
			if in.TotalShares.IsZero() || perShare(exact, in.TotalShares).Cmp(rat(math.MaxUint64)) <= 0 {
				t.Fatalf("CabalNAV = %v on a pot whose per-share price fits", err)
			}
			return
		}
		gap := new(big.Rat).Sub(exact, rat(got.Value.Uint64()))
		if gap.Sign() < 0 {
			t.Fatalf("Value %v exceeds exact %v", got.Value, exact.RatString())
		}
		if gap.Sign() > 0 && gap.Cmp(big.NewRat(int64(len(in.Holdings)), 1)) >= 0 {
			t.Fatalf("Value %v is %v under exact, a micro or more per holding", got.Value, gap.RatString())
		}
		if !in.TotalShares.IsZero() &&
			rat(got.PerShare.Uint64()).Cmp(perShare(rat(got.Value.Uint64()), in.TotalShares)) > 0 {
			t.Fatalf("PerShare %v exceeds the exact price of Value %v", got.PerShare, got.Value)
		}
	})
}

func TestProperty_ModifiedDietzWithNoFlowsIsTheSimpleReturn(t *testing.T) {
	t.Parallel()
	t1 := clock.Real{}.Now().UTC()
	rapid.Check(t, func(t *rapid.T) {
		start := rapid.Uint64Range(0, 1<<40).Draw(t, "start")
		end := rapid.Uint64Range(0, 1<<40).Draw(t, "end")
		span := time.Duration(rapid.Int64Range(1, int64(365*24*time.Hour)).Draw(t, "span"))
		gain, ret, err := domain.ModifiedDietz(
			domain.DietzInput{T0: t1.Add(-span), T1: t1, Start: usd(start), End: usd(end)},
		)
		want := new(big.Int).Sub(u(end), u(start))
		if err != nil || gain.Int64() != want.Int64() {
			t.Fatalf("ModifiedDietz gain = %v, %v, want %v", gain, err, want)
		}
		if start == 0 {
			if ret != nil {
				t.Fatalf("ModifiedDietz ret = %d from an empty start, want nil", *ret)
			}
			return
		}
		simple := want.Div(want.Mul(want, big.NewInt(10_000)), u(start))
		if ret == nil || int64(*ret) != simple.Int64() {
			t.Fatalf("ModifiedDietz ret = %v, want %v", fmtBps(ret), simple)
		}
	})
}

func TestProperty_RankIgnoresInputOrder(t *testing.T) {
	t.Parallel()
	base := clock.Real{}.Now().UTC()
	rapid.Check(t, func(t *rapid.T) {
		ids := rapid.SliceOfNDistinct(rapid.StringMatching(`[a-z]{1,4}`), 0, 30, rapid.ID[string]).Draw(t, "ids")
		cands := make([]domain.Candidate, len(ids))
		for i, id := range ids {
			cands[i] = domain.Candidate{
				SubjectID: id,
				CreatedAt: base.Add(time.Duration(rapid.IntRange(0, 3).Draw(t, "age")) * time.Second),
			}
			if rapid.Bool().Draw(t, "ranked") {
				cands[i].Return = bps(domain.Bps(rapid.Int64Range(-3, 3).Draw(t, "return")))
			}
		}
		keep := rapid.Bool().Draw(t, "keep_unranked")
		want := domain.Rank(cands, keep)
		got := domain.Rank(rapid.Permutation(cands).Draw(t, "order"), keep)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Rank of a permutation = %+v, want %+v", got, want)
		}
	})
}

func drawNAVInput(t *rapid.T) (domain.NAVInput, *big.Rat) {
	in := domain.NAVInput{
		USDC:        usd(rapid.Uint64Range(0, 1<<50).Draw(t, "usdc")),
		TotalShares: shares(rapid.Uint64Range(0, 1<<50).Draw(t, "total_shares")),
	}
	exact := rat(in.USDC.Uint64())
	for range rapid.IntRange(0, 5).Draw(t, "holdings") {
		units := rapid.Uint64Range(0, 1<<32).Draw(t, "units")
		decimals := rapid.Uint8Range(0, 19).Draw(t, "decimals")
		h := domain.Holding{
			Units: money.NewBaseUnits(units, decimals),
			Price: usd(rapid.Uint64Range(0, 1<<24).Draw(t, "price")),
		}
		in.Holdings = append(in.Holdings, h)
		scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
		exact.Add(exact, new(big.Rat).SetFrac(new(big.Int).Mul(u(units), u(h.Price.Uint64())), scale))
	}
	return in, exact
}

func perShare(value *big.Rat, total money.SharesUnits) *big.Rat {
	return new(big.Rat).Quo(new(big.Rat).Mul(value, rat(1_000_000)), rat(total.Uint64()))
}

func rat(v uint64) *big.Rat { return new(big.Rat).SetInt(u(v)) }

func u(v uint64) *big.Int { return new(big.Int).SetUint64(v) }
