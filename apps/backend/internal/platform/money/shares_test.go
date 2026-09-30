package money_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestParseSharesUnits(t *testing.T) {
	t.Parallel()
	got, err := money.ParseSharesUnits("18446744073709551615")
	if err != nil || got.Uint64() != math.MaxUint64 || got.String() != "18446744073709551615" {
		t.Fatalf("ParseSharesUnits(max) = %v, %v", got, err)
	}
	for _, raw := range []string{"", "-1", "01", "+1", "1.5", "18446744073709551616"} {
		_, err := money.ParseSharesUnits(raw)
		wantCode(t, err, errs.CodeInvalidInput)
	}
}

func TestSharesUnitsArithmetic(t *testing.T) {
	t.Parallel()
	a, b := money.SharesUnitsFromUint64(3_000_000), money.SharesUnitsFromUint64(1_000_000)
	if sum, err := a.Add(b); err != nil || sum.Uint64() != 4_000_000 {
		t.Fatalf("Add = %v, %v", sum, err)
	}
	if diff, err := a.Sub(b); err != nil || diff.Uint64() != 2_000_000 {
		t.Fatalf("Sub = %v, %v", diff, err)
	}
	_, err := money.SharesUnitsFromUint64(math.MaxUint64).Add(b)
	wantCode(t, err, errs.CodeInvalidInput)
	_, err = b.Sub(a)
	wantCode(t, err, errs.CodeInvalidInput)
	if !money.SharesUnitsFromUint64(0).IsZero() || a.IsZero() {
		t.Fatal("IsZero")
	}
}

func TestSharesUnitsEncodeLikeMicros(t *testing.T) {
	t.Parallel()
	type row struct {
		Shares money.SharesUnits `json:"shares"`
		Micros money.Micros      `json:"micros"`
	}
	in := row{Shares: money.SharesUnitsFromUint64(1_500_000), Micros: money.MicrosFromUint64(1_500_000)}
	raw, err := json.Marshal(in)
	if err != nil || string(raw) != `{"shares":"1500000","micros":"1500000"}` {
		t.Fatalf("json = %s, %v", raw, err)
	}
	var back row
	if err := json.Unmarshal(raw, &back); err != nil || back != in {
		t.Fatalf("round trip = %+v, %v", back, err)
	}
	wantCode(t, json.Unmarshal([]byte(`{"shares":"-1"}`), &back), errs.CodeInvalidInput)
}

func TestSharesUnitsSQLMatchesMicros(t *testing.T) {
	t.Parallel()
	shares, micros := money.SharesUnitsFromUint64(1_500_000), money.MicrosFromUint64(1_500_000)
	sv, err := shares.Value()
	mv, _ := micros.Value()
	if err != nil || sv != mv {
		t.Fatalf("Value = %v, %v, want %v", sv, err, mv)
	}
	var fromString, fromBytes money.SharesUnits
	if err := fromString.Scan(sv); err != nil || fromString != shares {
		t.Fatalf("Scan(string) = %v, %v", fromString, err)
	}
	if err := fromBytes.Scan([]byte("42")); err != nil || fromBytes.Uint64() != 42 {
		t.Fatalf("Scan([]byte) = %v, %v", fromBytes, err)
	}
	for _, src := range []any{nil, int64(5), "-1", "1.0"} {
		s := money.SharesUnitsFromUint64(9)
		wantCode(t, s.Scan(src), errs.CodeDecodeFailed)
		if s.Uint64() != 9 {
			t.Fatalf("Scan(%#v) changed the value to %v", src, s)
		}
	}
}
