package trading_test

import (
	"math"
	"testing"
)

const pastInt64 = "25000000000000000000"

func TestWidenedAmounts_aSwapAtTheInt64LimitRoundTripsThroughTheTableAndTheView(t *testing.T) {
	t.Parallel()
	d := newSwapDB(t)
	row := d.created(d.ids.NewV7(), usdcMint)
	row.InAmount = math.MaxInt64
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
	if got.InAmount != math.MaxInt64 || !got.OutAmount.Valid || got.OutAmount.Int64 != math.MaxInt64 {
		t.Fatalf("view amounts = %d, %+v, want int64 max in both", got.InAmount, got.OutAmount)
	}
}

func TestWidenedAmounts_theSwapColumnsHoldPastInt64AndAReadThatNarrowsThemFailsLoudly(t *testing.T) {
	t.Parallel()
	d := newSwapDB(t)
	row := d.created(d.ids.NewV7(), usdcMint)
	d.insert(t, row)
	if _, err := d.pool.Exec(
		t.Context(),
		`UPDATE swaps SET in_amount = $2::numeric, quote_out_amount = $2::numeric, out_amount = $2::numeric WHERE id = $1`,
		row.ID,
		pastInt64,
	); err != nil {
		t.Fatalf("a 25-token amount at 18 decimals must fit the column: %v", err)
	}

	var in string
	if err := d.pool.QueryRow(t.Context(), `SELECT in_amount::text FROM swaps WHERE id = $1`, row.ID).
		Scan(&in); err != nil ||
		in != pastInt64 {
		t.Fatalf("stored in_amount = %q, %v, want %s", in, err, pastInt64)
	}
	if got, err := d.q.SwapByID(t.Context(), row.ID); err == nil {
		t.Fatalf("SwapByID = %+v, want an error: an amount past int64 must not narrow silently", got)
	}
	if got, err := d.q.SwapDetail(t.Context(), row.ID); err == nil {
		t.Fatalf("SwapDetail = %+v, want an error: an amount past int64 must not narrow silently", got)
	}
}
