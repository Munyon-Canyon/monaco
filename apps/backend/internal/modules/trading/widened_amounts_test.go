package trading_test

import (
	"math"
	"strconv"
	"testing"
)

const pastInt64 = "25000000000000000000"

func TestWidenedAmounts_aSwapAtTheInt64LimitRoundTripsThroughTheTableAndTheView(t *testing.T) {
	t.Parallel()
	d := newSwapDB(t)
	limit := strconv.FormatInt(math.MaxInt64, 10)
	row := d.created(d.ids.NewV7(), usdcMint)
	row.InAmount = limit
	d.insert(t, row)
	d.submit(t, row.ID, "req-"+row.ID.String(), "sig-"+row.ID.String())
	if _, err := d.pool.Exec(
		t.Context(),
		`UPDATE swaps SET out_amount = $2 WHERE id = $1`,
		row.ID,
		int64(math.MaxInt64),
	); err != nil {
		t.Fatal(err)
	}

	got, err := d.q.SwapByID(t.Context(), row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.InAmount != limit || got.OutAmount != limit {
		t.Fatalf("view amounts = %s, %s, want int64 max in both", got.InAmount, got.OutAmount)
	}
}

func TestWidenedAmounts_theSwapColumnsHoldPastInt64AndEveryReadReturnsThemExactly(t *testing.T) {
	t.Parallel()
	d := newSwapDB(t)
	row := d.created(d.ids.NewV7(), usdcMint)
	d.insert(t, row)
	d.submit(t, row.ID, "req-"+row.ID.String(), "sig-"+row.ID.String())
	if _, err := d.pool.Exec(
		t.Context(),
		`UPDATE swaps SET in_amount = $2::numeric, quote_out_amount = $2::numeric, out_amount = $2::numeric,
			status = 'confirmed', confirmed_at = $3 WHERE id = $1`,
		row.ID,
		pastInt64,
		d.now,
	); err != nil {
		t.Fatalf("a 25-token amount at 18 decimals must fit the column: %v", err)
	}

	var in string
	if err := d.pool.QueryRow(t.Context(), `SELECT in_amount::text FROM swaps WHERE id = $1`, row.ID).
		Scan(&in); err != nil ||
		in != pastInt64 {
		t.Fatalf("stored in_amount = %q, %v, want %s", in, err, pastInt64)
	}
	byID, err := d.q.SwapByID(t.Context(), row.ID)
	if err != nil || byID.InAmount != pastInt64 || byID.OutAmount != pastInt64 {
		t.Fatalf("SwapByID = %+v, %v, want %s in both amounts", byID, err, pastInt64)
	}
	wantDetailAmounts(t, d, row.ID, pastInt64, pastInt64)
	wantViewAmounts(t, d, row.ID, pastInt64, pastInt64)
}
