package trading_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	twentyFiveTokens = "25000000000000000000"
	twelveTokens     = "12000000000000000000"
	twoToThe64       = "18446744073709551616"
	twoToThe256Less1 = "115792089237316195423570985008687907853269984665640564039457584007913129639935"
	wideUSDC         = "3000000000"
)

func (d swapDB) wide(action, inAmount, quoteOut string) sqlc.InsertCreatedParams {
	inMint, outMint, outDecimals := usdcMint, aaplxMint, int16(18)
	if action == "sell" {
		inMint, outMint = aaplxMint, usdcMint
		outDecimals = 6
	}
	return sqlc.InsertCreatedParams{
		ID: d.ids.NewV7(), SourceKind: "proposal", SourceID: d.ids.NewV7(), CabalID: d.ids.NewV7(),
		TreasuryAddress: "9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin", Action: action, Symbol: "AAPLx",
		InMint: inMint, OutMint: outMint, OutDecimals: outDecimals, InAmount: inAmount, QuoteOutAmount: quoteOut,
		SlippageBps: 100, SourceBatchSize: 1, CreatedAt: d.now,
	}
}

func (d swapDB) settleWide(t *testing.T, row sqlc.InsertCreatedParams, out string) string {
	t.Helper()
	d.insert(t, row)
	sig := "sig-" + row.ID.String()
	d.submit(t, row.ID, "req-"+row.ID.String(), sig)
	n, err := d.q.FinishConfirmed(t.Context(), sqlc.FinishConfirmedParams{
		ID: row.ID, OutAmount: out, FeeMicros: 5_000, ConfirmedAt: d.now,
	})
	if err != nil || n != 1 {
		t.Fatalf("FinishConfirmed = %d, %v, want 1 row", n, err)
	}
	return sig
}

func mustUnits(t *testing.T, raw string) money.Units {
	t.Helper()
	v, err := money.ParseUnits(raw)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func wantStoredAmounts(t *testing.T, d swapDB, id uuid.UUID, in, quote, out string) {
	t.Helper()
	var gotIn, gotQuote, gotOut string
	err := d.pool.QueryRow(
		t.Context(),
		`SELECT in_amount::text, quote_out_amount::text, out_amount::text FROM swaps WHERE id = $1`, id,
	).Scan(&gotIn, &gotQuote, &gotOut)
	if err != nil {
		t.Fatal(err)
	}
	if gotIn != in || gotQuote != quote || gotOut != out {
		t.Fatalf("stored amounts = %s, %s, %s, want %s, %s, %s", gotIn, gotQuote, gotOut, in, quote, out)
	}
}

func wantViewAmounts(t *testing.T, d swapDB, id uuid.UUID, in, out string) {
	t.Helper()
	view, err := app.NewQueries(d.pool).Swap(t.Context(), ids.SwapIDFrom(id))
	if err != nil {
		t.Fatal(err)
	}
	if !view.InAmount.Equal(mustUnits(t, in)) || !view.OutAmount.Equal(mustUnits(t, out)) {
		t.Fatalf("swap view amounts = %s, %s, want %s, %s", view.InAmount, view.OutAmount, in, out)
	}
	if view.Status != domain.StatusConfirmed {
		t.Fatalf("swap view status = %s, want confirmed", view.Status)
	}
}

func wantDetailAmounts(t *testing.T, d swapDB, id uuid.UUID, in, out string) {
	t.Helper()
	detail, err := d.q.SwapDetail(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if detail.InAmount != in || detail.OutAmount != out {
		t.Fatalf("swap detail amounts = %s, %s, want %s, %s", detail.InAmount, detail.OutAmount, in, out)
	}
}

func TestWideSwaps_a25TokenEVMBuySettlesThroughTheStoreWithoutLosingAUnit(t *testing.T) {
	t.Parallel()
	d := newSwapDB(t)
	row := d.wide("buy", wideUSDC, twentyFiveTokens)
	sig := d.settleWide(t, row, twentyFiveTokens)

	wantStoredAmounts(t, d, row.ID, wideUSDC, twentyFiveTokens, twentyFiveTokens)
	wantViewAmounts(t, d, row.ID, wideUSDC, twentyFiveTokens)
	wantDetailAmounts(t, d, row.ID, wideUSDC, twentyFiveTokens)
	bySig, err := d.q.SwapBySignature(t.Context(), sig)
	if err != nil || bySig.InAmount != wideUSDC || bySig.OutAmount != twentyFiveTokens {
		t.Fatalf("SwapBySignature = %+v, %v, want in %s out %s", bySig, err, wideUSDC, twentyFiveTokens)
	}
	latest, err := d.q.LatestBySource(t.Context(), sqlc.LatestBySourceParams{
		SourceKind: row.SourceKind, SourceID: row.SourceID,
	})
	if err != nil || latest.OutAmount != twentyFiveTokens {
		t.Fatalf("LatestBySource = %+v, %v, want out %s", latest, err, twentyFiveTokens)
	}
	retry, err := d.q.SwapForRetry(t.Context(), row.ID)
	if err != nil || retry.InAmount != wideUSDC || retry.QuoteOutAmount != twentyFiveTokens {
		t.Fatalf("SwapForRetry = %+v, %v, want in %s quote %s", retry, err, wideUSDC, twentyFiveTokens)
	}
}

func TestWideSwaps_a12TokenEVMSellSettlesThroughTheStoreWithoutLosingAUnit(t *testing.T) {
	t.Parallel()
	d := newSwapDB(t)
	row := d.wide("sell", twelveTokens, wideUSDC)
	sig := d.settleWide(t, row, wideUSDC)

	wantStoredAmounts(t, d, row.ID, twelveTokens, wideUSDC, wideUSDC)
	wantViewAmounts(t, d, row.ID, twelveTokens, wideUSDC)
	wantDetailAmounts(t, d, row.ID, twelveTokens, wideUSDC)
	forced, err := d.q.SwapForForceResolve(t.Context(), sig)
	if err != nil || forced.InAmount != twelveTokens {
		t.Fatalf("SwapForForceResolve = %+v, %v, want in %s", forced, err, twelveTokens)
	}
	retry, err := d.q.SwapForRetry(t.Context(), row.ID)
	if err != nil || retry.InAmount != twelveTokens || retry.QuoteOutAmount != wideUSDC {
		t.Fatalf("SwapForRetry = %+v, %v, want in %s quote %s", retry, err, twelveTokens, wideUSDC)
	}
}

func TestWideSwaps_aConfirmedSellIsListedWithItsFullRecordedAmount(t *testing.T) {
	t.Parallel()
	d := newSwapDB(t)
	row := d.wide("sell", twelveTokens, twentyFiveTokens)
	d.settleWide(t, row, twentyFiveTokens)

	sells, err := d.q.ListConfirmedSellsInto(t.Context(), usdcMint)
	if err != nil {
		t.Fatal(err)
	}
	for _, sell := range sells {
		if sell.ID == row.ID {
			if sell.OutAmount != twentyFiveTokens {
				t.Fatalf("recorded out amount = %s, want %s", sell.OutAmount, twentyFiveTokens)
			}
			return
		}
	}
	t.Fatalf("confirmed sell %s is not listed among %d sells", row.ID, len(sells))
}

func TestWideSwaps_aStuckSwapKeepsItsFullInAmount(t *testing.T) {
	t.Parallel()
	d := newSwapDB(t)
	row := d.wide("sell", twentyFiveTokens, wideUSDC)
	d.insert(t, row)
	d.submit(t, row.ID, "req-"+row.ID.String(), "sig-"+row.ID.String())

	stuck, err := d.q.StuckSwaps(t.Context(), sqlc.StuckSwapsParams{
		Cutoff: d.now.AddDate(0, 0, 1), RowLimit: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, swap := range stuck {
		if swap.ID == row.ID {
			if swap.InAmount != twentyFiveTokens || swap.OutAmount != "" {
				t.Fatalf("stuck amounts = %q, %q, want %s and none", swap.InAmount, swap.OutAmount, twentyFiveTokens)
			}
			return
		}
	}
	t.Fatalf("submitted swap %s is not listed among %d stuck swaps", row.ID, len(stuck))
}

func TestWideSwaps_theStoreRoundTripsExactlyAtTheEdgesOfUint64AndUint256(t *testing.T) {
	t.Parallel()
	for name, edge := range map[string]string{
		"2^64":      twoToThe64,
		"2^256 - 1": twoToThe256Less1,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newSwapDB(t)
			row := d.wide("buy", edge, edge)
			d.settleWide(t, row, edge)

			wantStoredAmounts(t, d, row.ID, edge, edge, edge)
			wantViewAmounts(t, d, row.ID, edge, edge)
			wantDetailAmounts(t, d, row.ID, edge, edge)
		})
	}
}

func TestWideSwaps_aStoredAmountFromAnUnconfirmedSwapReadsBackWithNoOutAmount(t *testing.T) {
	t.Parallel()
	d := newSwapDB(t)
	row := d.wide("buy", wideUSDC, twentyFiveTokens)
	d.insert(t, row)

	got, err := d.q.SwapByID(t.Context(), row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.InAmount != wideUSDC || got.OutAmount != "" {
		t.Fatalf("view amounts = %q, %q, want %s and none", got.InAmount, got.OutAmount, wideUSDC)
	}
}

func sameJSON(a, b any) bool {
	left, leftErr := json.Marshal(a)
	right, rightErr := json.Marshal(b)
	return leftErr == nil && rightErr == nil && string(left) == string(right)
}

func TestWideSwaps_theEVMVenueReadsA25TokenTransferExactly(t *testing.T) {
	t.Parallel()
	const hex25 = "15af1d78b58c40000"
	fill, err := newVenueEnv(t).fill(t, receiptReply("0x1", transferLog(venueToken, venueOwner, hex25)), 0)
	if err != nil || fill.State != app.FillSucceeded || !fill.Out.Equal(mustUnits(t, twentyFiveTokens)) {
		t.Fatalf("Fill = %+v, %v, want %s received", fill, err, twentyFiveTokens)
	}
	inspected, err := newVenueEnv(t).inspect(t, receiptReply("0x1", transferLog(venueToken, venueOwner, hex25)))
	if err != nil || !inspected.Out.Equal(mustUnits(t, twentyFiveTokens)) {
		t.Fatalf("Inspect = %+v, %v, want %s received", inspected, err, twentyFiveTokens)
	}
}
