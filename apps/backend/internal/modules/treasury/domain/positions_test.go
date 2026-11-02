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

func sameDelta(a, b domain.CabalPositionDelta) bool {
	return a.Asset == b.Asset && a.Units.Equal(b.Units) && a.CostIn == b.CostIn
}

func leg(account domain.CabalAccount, asset domain.Asset, v int64) domain.CabalEntry {
	return domain.CabalEntry{Account: account, Asset: asset, Amount: units(v)}
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
		{Asset: usdc, Units: units(-50)},
		{Asset: aapl, Units: units(21), CostIn: money.MicrosFromUint64(50)},
	}
	if err != nil || len(got) != 2 || !sameDelta(got[0], want[0]) || !sameDelta(got[1], want[1]) {
		t.Fatalf("buy deltas = %+v, %v, want %+v", got, err, want)
	}
	sell := cabalTxn(t,
		leg(domain.CabalTreasury, aapl, -7), leg(domain.CabalVenue, aapl, 7),
		leg(domain.CabalTreasury, usdc, 20), leg(domain.CabalVenue, usdc, -20),
	)
	got, err = sell.PositionDeltas(usdc)
	want = []domain.CabalPositionDelta{
		{Asset: usdc, Units: units(20), CostIn: money.MicrosFromUint64(20)},
		{Asset: aapl, Units: units(-7)},
	}
	if err != nil || len(got) != 2 || !sameDelta(got[0], want[0]) || !sameDelta(got[1], want[1]) {
		t.Fatalf("sell deltas = %+v, %v, want %+v", got, err, want)
	}
}

func wantDeltas(t *testing.T, name string, txn domain.CabalTxn, want ...domain.CabalPositionDelta) {
	t.Helper()
	got, err := txn.PositionDeltas(usdc)
	if err != nil || len(got) != len(want) {
		t.Fatalf("%s deltas = %+v, %v, want %+v", name, got, err, want)
	}
	for i := range want {
		if !sameDelta(got[i], want[i]) {
			t.Fatalf("%s deltas = %+v, want %+v", name, got, want)
		}
	}
}

func TestPositionDeltas_costsABridgedEVMBuyAtTheEVMCashItSpent(t *testing.T) {
	t.Parallel()
	const (
		baseUSDC = domain.Asset("base:0x833589fcd6edb6e08f4c7c32d4f71b54bda02913")
		rhUSDG   = domain.Asset("robinhood:0x5fc5360d0400a0fd4f2af552add042d716f1d168")
		baseAAPL = domain.Asset("base:0xaa00000000000000000000000000000000000001")
		rhAAPL   = domain.Asset("robinhood:0xaa00000000000000000000000000000000000002")
	)
	micros := money.MicrosFromUint64
	wantDeltas(t, "bridge out", cabalTxn(t,
		leg(domain.CabalTreasury, usdc, -100_000_000), leg(domain.CabalVenue, usdc, 100_000_000),
	), domain.CabalPositionDelta{Asset: usdc, Units: units(-100_000_000)})
	wantDeltas(t, "bridge landed", cabalTxn(t,
		leg(domain.CabalVenue, baseUSDC, -99_925_000), leg(domain.CabalTreasury, baseUSDC, 99_925_000),
	), domain.CabalPositionDelta{Asset: baseUSDC, Units: units(99_925_000), CostIn: micros(99_925_000)})
	wantDeltas(t, "evm buy", cabalTxn(t,
		leg(domain.CabalTreasury, baseUSDC, -99_925_000), leg(domain.CabalVenue, baseUSDC, 99_925_000),
		leg(domain.CabalTreasury, baseAAPL, 400_000_000), leg(domain.CabalVenue, baseAAPL, -400_000_000),
	),
		domain.CabalPositionDelta{Asset: baseUSDC, Units: units(-99_925_000)},
		domain.CabalPositionDelta{Asset: baseAAPL, Units: units(400_000_000), CostIn: micros(99_925_000)},
	)
	wantDeltas(t, "evm buy with a solana fee", cabalTxn(t,
		leg(domain.CabalTreasury, rhUSDG, -40_000_000), leg(domain.CabalVenue, rhUSDG, 40_000_000),
		leg(domain.CabalTreasury, rhAAPL, 3_000), leg(domain.CabalVenue, rhAAPL, -3_000),
		leg(domain.CabalTreasury, usdc, -5_000), leg(domain.CabalFees, usdc, 5_000),
	),
		domain.CabalPositionDelta{Asset: usdc, Units: units(-5_000)},
		domain.CabalPositionDelta{Asset: rhUSDG, Units: units(-40_000_000)},
		domain.CabalPositionDelta{Asset: rhAAPL, Units: units(3_000), CostIn: micros(40_005_000)},
	)
	wantDeltas(t, "evm sell", cabalTxn(t,
		leg(domain.CabalTreasury, baseAAPL, -400_000_000), leg(domain.CabalVenue, baseAAPL, 400_000_000),
		leg(domain.CabalTreasury, baseUSDC, 101_000_000), leg(domain.CabalVenue, baseUSDC, -101_000_000),
	),
		domain.CabalPositionDelta{Asset: baseUSDC, Units: units(101_000_000), CostIn: micros(101_000_000)},
		domain.CabalPositionDelta{Asset: baseAAPL, Units: units(-400_000_000)},
	)
	wantDeltas(t, "a token on another network's cash address is not cash", cabalTxn(t,
		leg(domain.CabalTreasury, usdc, -7), leg(domain.CabalVenue, usdc, 7),
		leg(domain.CabalTreasury, "arbitrum:0x833589fcd6edb6e08f4c7c32d4f71b54bda02913", 9),
		leg(domain.CabalVenue, "arbitrum:0x833589fcd6edb6e08f4c7c32d4f71b54bda02913", -9),
	),
		domain.CabalPositionDelta{Asset: usdc, Units: units(-7)},
		domain.CabalPositionDelta{
			Asset: "arbitrum:0x833589fcd6edb6e08f4c7c32d4f71b54bda02913", Units: units(9), CostIn: micros(7),
		},
	)
}

func TestPositionDeltas_solanaOnlyTxnsKeepTodaysDeltas(t *testing.T) {
	t.Parallel()
	micros := money.MicrosFromUint64
	wantDeltas(t, "buy with a fee", cabalTxn(t,
		leg(domain.CabalTreasury, usdc, -25_000_000), leg(domain.CabalVenue, usdc, 25_000_000),
		leg(domain.CabalTreasury, usdc, -5_000), leg(domain.CabalFees, usdc, 5_000),
		leg(domain.CabalTreasury, tsla, 105_000_000), leg(domain.CabalVenue, tsla, -105_000_000),
	),
		domain.CabalPositionDelta{Asset: usdc, Units: units(-25_005_000)},
		domain.CabalPositionDelta{Asset: tsla, Units: units(105_000_000), CostIn: micros(25_005_000)},
	)
	wantDeltas(t, "fund", cabalTxn(t,
		leg(domain.CabalMembers, usdc, -30_000_000), leg(domain.CabalTreasury, usdc, 30_000_000),
	), domain.CabalPositionDelta{Asset: usdc, Units: units(30_000_000), CostIn: micros(30_000_000)})
	wantDeltas(t, "cash out", cabalTxn(t,
		leg(domain.CabalTreasury, usdc, -12_000_000), leg(domain.CabalMembers, usdc, 12_000_000),
	), domain.CabalPositionDelta{Asset: usdc, Units: units(-12_000_000)})
	wantDeltas(t, "airdrop", cabalTxn(t,
		leg(domain.CabalTreasury, aapl, 4), leg(domain.CabalVenue, aapl, -4),
	), domain.CabalPositionDelta{Asset: aapl, Units: units(4)})
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
	_, err := two.PositionDeltas(usdc)
	wantCode(t, err, errs.CodeInvalidInput)
	pastInt64 := cabalTxn(t,
		leg(domain.CabalTreasury, aapl, math.MaxInt64), leg(domain.CabalTreasury, aapl, 1),
		leg(domain.CabalVenue, aapl, math.MinInt64),
	)
	wantDeltas(t, "a delta past int64", pastInt64,
		domain.CabalPositionDelta{Asset: aapl, Units: mustSignedUnits(t, "9223372036854775808")})
}

func mustSignedUnits(tb testing.TB, raw string) money.SignedUnits {
	tb.Helper()
	v, err := money.ParseSignedUnits(raw)
	if err != nil {
		tb.Fatal(err)
	}
	return v
}

func TestPositionDeltas_keepsTokenLegsPastInt64AndUint64Exact(t *testing.T) {
	t.Parallel()
	tenTokens := mustSignedUnits(t, "10000000000000000000")
	twentyFive := mustSignedUnits(t, "25000000000000000000")
	twelve := mustSignedUnits(t, "12000000000000000000")
	token := func(account domain.CabalAccount, amount money.SignedUnits) domain.CabalEntry {
		return domain.CabalEntry{Account: account, Asset: aapl, Amount: amount}
	}
	for name, tc := range map[string]struct {
		amount money.SignedUnits
		cost   int64
	}{"a 10 token buy": {tenTokens, 50_000_000}, "a 25 token buy": {twentyFive, 125_000_000}} {
		wantDeltas(t, name, cabalTxn(t,
			leg(domain.CabalTreasury, usdc, -tc.cost), leg(domain.CabalVenue, usdc, tc.cost),
			token(domain.CabalTreasury, tc.amount), token(domain.CabalVenue, tc.amount.Neg()),
		),
			domain.CabalPositionDelta{Asset: usdc, Units: units(-tc.cost)},
			domain.CabalPositionDelta{Asset: aapl, Units: tc.amount, CostIn: micros(tc.cost)},
		)
	}
	wantDeltas(t, "a 12 token sell", cabalTxn(t,
		token(domain.CabalTreasury, twelve.Neg()), token(domain.CabalVenue, twelve),
		leg(domain.CabalTreasury, usdc, 60_000_000), leg(domain.CabalVenue, usdc, -60_000_000),
	),
		domain.CabalPositionDelta{Asset: usdc, Units: units(60_000_000), CostIn: money.MicrosFromUint64(60_000_000)},
		domain.CabalPositionDelta{Asset: aapl, Units: twelve.Neg()},
	)
}

func TestPositionDeltas_rejectsACashDeltaPastUint64(t *testing.T) {
	t.Parallel()
	over := mustSignedUnits(t, "18446744073709551616")
	cash := func(account domain.CabalAccount, amount money.SignedUnits) domain.CabalEntry {
		return domain.CabalEntry{Account: account, Asset: usdc, Amount: amount}
	}
	txn := cabalTxn(t, cash(domain.CabalTreasury, over), cash(domain.CabalVenue, over.Neg()))
	_, err := txn.PositionDeltas(usdc)
	wantCode(t, err, errs.CodeInvalidInput)
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
	_, err = huge.PositionDelta()
	wantCode(t, err, errs.CodeInvalidInput)
}

func TestUserPositionDelta_refusesContributionsAndWithdrawalsPastUint64(t *testing.T) {
	t.Parallel()
	cabal := cabalID(t, 6)
	legs := func(dir int64) []domain.UserEntry {
		out := make([]domain.UserEntry, 0, 8)
		for range 4 {
			out = append(out,
				domain.UserEntry{Account: domain.UserCabal, Asset: usdc, Amount: amount(dir * (1 << 62))},
				domain.UserEntry{Account: domain.UserWallet, Asset: usdc, Amount: amount(-dir * (1 << 62))},
			)
		}
		return out
	}
	for name, sign := range map[string]int64{"contributed": 1, "withdrawn": -1} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			txn, err := domain.NewUserTxn(domain.UserTxnHeader{CabalID: cabal, Kind: domain.UserFund}, legs(sign))
			if err != nil {
				t.Fatal(err)
			}
			_, err = txn.PositionDelta()
			wantCode(t, err, errs.CodeInvalidInput)
		})
	}
}
