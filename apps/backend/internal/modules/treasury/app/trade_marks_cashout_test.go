package app

import "testing"

func TestReplayTrades_aCashOutSaleMovesTheCostBasisButMakesNoMark(t *testing.T) {
	t.Parallel()
	sale := marksTrade(1, sideSell, 5, 600)
	sale.cashOut = true
	marks, err := replayTrades([]trade{marksTrade(0, sideBuy, 10, 1000), sale, marksTrade(2, sideSell, 5, 700)},
		AssetName{Decimals: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(marks) != 2 || marks[1].CostOfSoldMicros.Uint64() != 500 {
		t.Fatalf(
			"marks = %+v, want the buy and the last sale, which sold half of the cost left after the cash-out",
			marks,
		)
	}
}
