package domain_test

import (
	"math/big"
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func drawBalanced(t *rapid.T) []domain.CabalEntry {
	assets := []domain.Asset{usdc, "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp", "shares:x"}
	accounts := []domain.CabalAccount{domain.CabalTreasury, domain.CabalVenue, domain.CabalMembers, domain.CabalFees}
	legs := rapid.IntRange(1, 6).Draw(t, "legs")
	out := make([]domain.CabalEntry, 0, 2*legs)
	for range legs {
		asset := rapid.SampledFrom(assets).Draw(t, "asset")
		v := rapid.Int64Range(1, 1<<50).Draw(t, "amount")
		out = append(out,
			domain.CabalEntry{Account: rapid.SampledFrom(accounts).Draw(t, "from"), Asset: asset, Amount: units(-v)},
			domain.CabalEntry{Account: rapid.SampledFrom(accounts).Draw(t, "to"), Asset: asset, Amount: units(v)},
		)
	}
	return rapid.Permutation(out).Draw(t, "order")
}

func zeroPerAsset(entries []domain.CabalEntry) bool {
	sums := map[domain.Asset]*big.Int{}
	for _, e := range entries {
		if sums[e.Asset] == nil {
			sums[e.Asset] = new(big.Int)
		}
		sums[e.Asset].Add(sums[e.Asset], e.Amount.Big())
	}
	for _, s := range sums {
		if s.Sign() != 0 {
			return false
		}
	}
	return true
}

func TestLedgerProperty_everyAcceptedHeaderSumsToZeroPerAsset(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		entries := drawBalanced(t)
		if rapid.Bool().Draw(t, "perturb") {
			i := rapid.IntRange(0, len(entries)-1).Draw(t, "entry")
			entries[i].Amount = units(entries[i].Amount.Int64OrZero() + rapid.Int64Range(1, 1000).Draw(t, "delta"))
		}
		txn, err := domain.NewCabalTxn(domain.CabalTxnHeader{Kind: domain.CabalSwap}, entries)
		switch {
		case !zeroPerAsset(entries):
			code := errs.CodeOf(err)
			if err == nil || (code != errs.CodeLedgerUnbalanced && code != errs.CodeInvalidInput) {
				t.Fatalf("unbalanced header = %v, want ledger_unbalanced", err)
			}
		case err != nil:
			t.Fatalf("balanced header rejected: %v", err)
		case !zeroPerAsset(txn.Entries()):
			t.Fatalf("accepted header does not sum to zero: %+v", txn.Entries())
		}
	})
}

func drawPot(t *rapid.T) (money.Micros, money.SharesUnits, money.Micros) {
	in := money.MicrosFromUint64(rapid.Uint64Range(0, 1<<40).Draw(t, "in"))
	total := money.SharesUnitsFromUint64(rapid.Uint64Range(1, 1<<40).Draw(t, "total"))
	pot := money.MicrosFromUint64(rapid.Uint64Range(1<<20, 1<<40).Draw(t, "pot"))
	return in, total, pot
}

func mint(t *rapid.T, in money.Micros, total money.SharesUnits, pot money.Micros) money.SharesUnits {
	minted, err := domain.MintShares(in, total, pot)
	if err != nil {
		t.Fatal(err)
	}
	return minted
}

func TestSharesProperty_mintingNeverMintsValue(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		in, total, pot := drawPot(t)
		minted := mint(t, in, total, pot)
		after, _ := total.Add(minted)
		potAfter, _ := pot.Add(in)
		payout, err := domain.PayoutFor(minted, after, potAfter)
		if err != nil || payout.Cmp(in) > 0 {
			t.Fatalf("minted %v for %v into %v/%v; they pay out %v, %v", minted, in, total, pot, payout, err)
		}
	})
}

func TestSharesProperty_roundingFavoursThePot(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		in, total, pot := drawPot(t)
		minted := mint(t, in, total, pot)
		after, _ := total.Add(minted)
		potAfter, _ := pot.Add(in)
		if held, _ := domain.PayoutFor(total, after, potAfter); held.Cmp(pot) < 0 {
			t.Fatalf("existing holders fell from %v to %v when %v joined", pot, held, in)
		}
		units := money.SharesUnitsFromUint64(rapid.Uint64Range(0, total.Uint64()).Draw(t, "units"))
		payout, err := domain.PayoutFor(units, total, pot)
		if err != nil {
			t.Fatal(err)
		}
		paid := new(big.Int).Mul(new(big.Int).SetUint64(payout.Uint64()), new(big.Int).SetUint64(total.Uint64()))
		owed := new(big.Int).Mul(new(big.Int).SetUint64(units.Uint64()), new(big.Int).SetUint64(pot.Uint64()))
		if paid.Cmp(owed) > 0 {
			t.Fatalf("PayoutFor(%v, %v, %v) = %v pays more than the exact share", units, total, pot, payout)
		}
	})
}

func drawWideUnits(t *rapid.T, label string) money.Units {
	raw := rapid.SliceOfN(rapid.Byte(), 1, 32).Draw(t, label)
	v, err := money.UnitsFromBig(new(big.Int).SetBytes(raw))
	if err != nil || v.IsZero() {
		t.Skip()
	}
	return v
}

func TestLedgerProperty_wideTokenLegsBalanceExactlyOrAreRefused(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		v := drawWideUnits(t, "amount")
		entries := []domain.CabalEntry{
			{Account: domain.CabalTreasury, Asset: aapl, Amount: v.Signed()},
			{Account: domain.CabalVenue, Asset: aapl, Amount: v.Signed().Neg()},
		}
		if rapid.Bool().Draw(t, "perturb") {
			extra := money.SignedUnitsFromInt64(rapid.Int64Range(1, 1000).Draw(t, "delta"))
			sum, err := entries[0].Amount.Add(extra)
			if err != nil {
				t.Skip()
			}
			entries[0].Amount = sum
			_, err = domain.NewCabalTxn(domain.CabalTxnHeader{Kind: domain.CabalSwap}, entries)
			if errs.CodeOf(err) != errs.CodeLedgerUnbalanced {
				t.Fatalf("a %v drift on %v = %v, want ledger_unbalanced", extra, v, err)
			}
			return
		}
		txn, err := domain.NewCabalTxn(domain.CabalTxnHeader{Kind: domain.CabalSwap}, entries)
		if err != nil || !zeroPerAsset(txn.Entries()) {
			t.Fatalf("balanced %v legs = %v, want accepted", v, err)
		}
		deltas, err := txn.PositionDeltas(usdc)
		if err != nil || len(deltas) != 1 || !deltas[0].Units.Equal(v.Signed()) {
			t.Fatalf("deltas for %v = %+v, %v, want exactly the treasury leg", v, deltas, err)
		}
	})
}
