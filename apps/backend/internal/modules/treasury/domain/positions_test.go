package domain_test

import (
	"math"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	aapl = domain.Asset("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
	tsla = domain.Asset("XsDoVfqeBukxuZHWhdvWHBhgEHjGNst4MLodqsJHzoB")
)

func cabalTxn(t *testing.T, entries ...domain.CabalEntry) domain.CabalTxn {
	t.Helper()
	txn, err := domain.NewCabalTxn(domain.CabalTxnHeader{Kind: domain.CabalSwap}, entries)
	if err != nil {
		t.Fatal(err)
	}
	return txn
}

func leg(account domain.CabalAccount, asset domain.Asset, v int64) domain.CabalEntry {
	return domain.CabalEntry{Account: account, Asset: asset, Amount: amount(v)}
}

func TestPositionDeltas_costsABuyAtTheUSDCPaidAndReleasesNothingOnASell(t *testing.T) {
	t.Parallel()
	buy := cabalTxn(t,
		leg(domain.CabalTreasury, usdc, -50), leg(domain.CabalVenue, usdc, 50),
		leg(domain.CabalTreasury, aapl, 21), leg(domain.CabalVenue, aapl, -21),
		leg(domain.CabalFees, tsla, 3), leg(domain.CabalVenue, tsla, -3),
	)
	got, err := buy.PositionDeltas(usdc)
	want := []domain.CabalPositionDelta{
		{Asset: usdc, Units: amount(-50)},
		{Asset: aapl, Units: amount(21), CostIn: money.MicrosFromUint64(50)},
	}
	if err != nil || len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("buy deltas = %+v, %v, want %+v", got, err, want)
	}
	sell := cabalTxn(t,
		leg(domain.CabalTreasury, aapl, -7), leg(domain.CabalVenue, aapl, 7),
		leg(domain.CabalTreasury, usdc, 20), leg(domain.CabalVenue, usdc, -20),
	)
	got, err = sell.PositionDeltas(usdc)
	want = []domain.CabalPositionDelta{
		{Asset: usdc, Units: amount(20), CostIn: money.MicrosFromUint64(20)},
		{Asset: aapl, Units: amount(-7)},
	}
	if err != nil || len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("sell deltas = %+v, %v, want %+v", got, err, want)
	}
}

func TestPositionDeltas_skipsANetZeroAssetAndRejectsWhatItCannotCost(t *testing.T) {
	t.Parallel()
	wash := cabalTxn(t, leg(domain.CabalTreasury, aapl, 5), leg(domain.CabalTreasury, aapl, -5))
	if got, err := wash.PositionDeltas(usdc); err != nil || len(got) != 0 {
		t.Fatalf("wash deltas = %+v, %v, want none", got, err)
	}
	two := cabalTxn(t,
		leg(domain.CabalTreasury, usdc, -50), leg(domain.CabalVenue, usdc, 50),
		leg(domain.CabalTreasury, aapl, 1), leg(domain.CabalVenue, aapl, -1),
		leg(domain.CabalTreasury, tsla, 1), leg(domain.CabalVenue, tsla, -1),
	)
	if _, err := two.PositionDeltas(usdc); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("two buys for one payment = %v, want invalid_input", err)
	}
	huge := cabalTxn(t,
		leg(domain.CabalTreasury, aapl, math.MaxInt64), leg(domain.CabalTreasury, aapl, 1),
		leg(domain.CabalVenue, aapl, math.MinInt64),
	)
	if _, err := huge.PositionDeltas(usdc); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("overflowing deltas = %v, want invalid_input", err)
	}
}

func TestUserPositionDelta_countsSharesContributionsAndWithdrawals(t *testing.T) {
	t.Parallel()
	cabal := cabalID(t, 5)
	shares := domain.SharesAsset(cabal)
	user := func(account domain.UserAccount, asset domain.Asset, v int64) domain.UserEntry {
		return domain.UserEntry{Account: account, Asset: asset, Amount: amount(v)}
	}
	txn, err := domain.NewUserTxn(domain.UserTxnHeader{CabalID: cabal, Kind: domain.UserFund}, []domain.UserEntry{
		user(domain.UserWallet, usdc, -30), user(domain.UserCabal, usdc, 30),
		user(domain.UserCabal, usdc, -4), user(domain.UserWallet, usdc, 4),
		user(domain.UserHolder, shares, 26), user(domain.UserIssuer, shares, -26),
		user(domain.UserHolder, aapl, 9), user(domain.UserIssuer, aapl, -9),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := txn.PositionDelta()
	want := domain.UserPositionDelta{
		Shares: amount(26), Contributed: money.MicrosFromUint64(30), Withdrawn: money.MicrosFromUint64(4),
	}
	if err != nil || got != want {
		t.Fatalf("PositionDelta = %+v, %v, want %+v", got, err, want)
	}
	huge, err := domain.NewUserTxn(domain.UserTxnHeader{CabalID: cabal}, []domain.UserEntry{
		user(domain.UserHolder, shares, math.MaxInt64), user(domain.UserHolder, shares, 1),
		user(domain.UserIssuer, shares, math.MinInt64),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := huge.PositionDelta(); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("overflowing shares = %v, want invalid_input", err)
	}
}
