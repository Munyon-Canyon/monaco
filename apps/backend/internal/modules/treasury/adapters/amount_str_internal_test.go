package adapters

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func decodeNumbers(raw []byte, out *map[string]any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	return d.Decode(out)
}

func fieldsOf(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := decodeNumbers(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestWireActivity_aPastInt64UnitsFigureIsNullAsAnIntegerAndExactAsAString(t *testing.T) {
	t.Parallel()
	wide, err := money.ParseUnits("25000000000000000000")
	if err != nil {
		t.Fatal(err)
	}
	got, err := wireActivity(app.ActivityView{Kind: domain.ActivityFund, Units: &wide})
	if err != nil {
		t.Fatalf("wireActivity = %v", err)
	}
	fields := fieldsOf(t, got)
	if fields["units"] != nil || fields["units_str"] != "25000000000000000000" {
		t.Fatalf("units = %v, units_str = %v", fields["units"], fields["units_str"])
	}
	fits := money.UnitsFromUint64(105_000_000)
	got, err = wireActivity(app.ActivityView{Kind: domain.ActivityFund, Units: &fits})
	if err != nil {
		t.Fatalf("wireActivity = %v", err)
	}
	fields = fieldsOf(t, got)
	if fields["units"] != json.Number("105000000") || fields["units_str"] != "105000000" {
		t.Fatalf("units = %v, units_str = %v", fields["units"], fields["units_str"])
	}
	got, err = wireActivity(app.ActivityView{Kind: domain.ActivityFund})
	if err != nil {
		t.Fatalf("wireActivity = %v", err)
	}
	if fields = fieldsOf(t, got); fields["units"] != nil {
		t.Fatalf("units without a figure = %v, want null", fields["units"])
	} else if _, ok := fields["units_str"]; ok {
		t.Fatalf("units_str without a figure = %v, want it absent", fields["units_str"])
	}
}

func TestWireHolding_carriesTheTokenAmountAsAStringAndNeverFailsOnAWideSolanaFigure(t *testing.T) {
	t.Parallel()
	var w int64Wire
	got := wireHolding(&w, port.Holding{TokenAmount: money.NewBaseUnits(73_000_000, 8)})
	fields := fieldsOf(t, got)
	if fields["token_amount"] != json.Number("73000000") || fields["token_amount_str"] != "73000000" {
		t.Fatalf("token_amount = %v, token_amount_str = %v", fields["token_amount"], fields["token_amount_str"])
	}
	wide := wireHolding(&w, port.Holding{TokenAmount: money.NewBaseUnits(1<<63, 8)})
	fields = fieldsOf(t, wide)
	if fields["token_amount"] != json.Number("0") || fields["token_amount_str"] != "9223372036854775808" ||
		w.err != nil {
		t.Fatalf(
			"token_amount = %v, token_amount_str = %v, err = %v",
			fields["token_amount"],
			fields["token_amount_str"],
			w.err,
		)
	}
}
