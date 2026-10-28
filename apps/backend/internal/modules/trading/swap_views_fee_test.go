package trading_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func TestSwapViews_feeMicros(t *testing.T) {
	t.Parallel()
	d := newPortDB(t)
	row := d.created(d.ids.NewV7(), usdcMint)
	d.insert(t, row)
	id := ids.SwapIDFrom(row.ID)
	open, err := d.port.Swap(t.Context(), id)
	if err != nil || open.FeeMicros != nil {
		t.Fatalf("Swap before the fill = %+v, %v, want no fee", open, err)
	}
	d.submit(t, row.ID, "req-1", "sig-1")
	d.confirm(t, row.ID)
	reads := map[string]func() (*uint64, error){
		"Swap": func() (*uint64, error) {
			v, err := d.port.Swap(t.Context(), id)
			return feeOf(v.FeeMicros), err
		},
		"SwapBySignature": func() (*uint64, error) {
			v, err := d.port.SwapBySignature(t.Context(), "sig-1")
			return feeOf(v.FeeMicros), err
		},
		"LatestBySource": func() (*uint64, error) {
			v, _, err := d.port.LatestBySource(t.Context(), proposal(row.SourceID))
			return feeOf(v.FeeMicros), err
		},
	}
	for name, read := range reads {
		if got, err := read(); err != nil || got == nil || *got != 5_000 {
			t.Errorf("%s fee = %v, %v, want 5000 micros from the confirmed row", name, got, err)
		}
	}
}

func TestSwapViews_negativeFeeIsDecodeFailed(t *testing.T) {
	t.Parallel()
	d := newPortDB(t)
	id := d.corrupt(t, `fee_micros = -1`)
	if _, err := d.port.Swap(t.Context(), ids.SwapIDFrom(id)); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("Swap with a negative fee err = %v, want decode_failed", err)
	}
}

func feeOf(m interface{ Uint64() uint64 }) *uint64 {
	if m == nil {
		return nil
	}
	v := m.Uint64()
	return &v
}
