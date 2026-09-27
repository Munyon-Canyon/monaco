package money_test

import (
	"encoding/json"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type ledgerRow struct {
	Amount money.Micros       `json:"amount"`
	Delta  money.SignedMicros `json:"delta"`
}

func TestJSONEncodesDecimalStrings(t *testing.T) {
	t.Parallel()
	row := ledgerRow{Amount: money.MicrosFromUint64(1_500_000), Delta: money.SignedMicrosFromInt64(-250)}
	raw, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"amount":"1500000","delta":"-250"}`; string(raw) != want {
		t.Fatalf("json = %s, want %s", raw, want)
	}
	var back ledgerRow
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back != row {
		t.Fatalf("round trip = %+v, want %+v", back, row)
	}
}

func TestJSONRejectsNonCanonical(t *testing.T) {
	t.Parallel()
	bad := []string{
		`{"amount":1500000}`,
		`{"amount":"1.5"}`,
		`{"amount":"-1"}`,
		`{"amount":"01"}`,
		`{"delta":-250}`,
		`{"delta":"-0"}`,
		`{"delta":"+5"}`,
	}
	for _, raw := range bad {
		var row ledgerRow
		if err := json.Unmarshal([]byte(raw), &row); err == nil {
			t.Fatalf("Unmarshal(%s) accepted %+v", raw, row)
		}
	}
}

func TestSQLRoundTrip(t *testing.T) {
	t.Parallel()
	m := money.MicrosFromUint64(18_446_744_073_709_551_615)
	v, err := m.Value()
	if err != nil || v != "18446744073709551615" {
		t.Fatalf("Micros.Value() = %v, %v", v, err)
	}
	var fromString, fromBytes money.Micros
	if err := fromString.Scan(v); err != nil || fromString != m {
		t.Fatalf("Micros.Scan(string) = %v, %v", fromString, err)
	}
	if err := fromBytes.Scan([]byte("42")); err != nil || fromBytes.Uint64() != 42 {
		t.Fatalf("Micros.Scan([]byte) = %v, %v", fromBytes, err)
	}
}

func TestSignedSQLRoundTrip(t *testing.T) {
	t.Parallel()
	s := money.SignedMicrosFromInt64(-9_223_372_036_854_775_808)
	sv, err := s.Value()
	if err != nil || sv != "-9223372036854775808" {
		t.Fatalf("SignedMicros.Value() = %v, %v", sv, err)
	}
	var sBack money.SignedMicros
	if err := sBack.Scan(sv); err != nil || sBack != s {
		t.Fatalf("SignedMicros.Scan(string) = %v, %v", sBack, err)
	}
	if err := sBack.Scan([]byte("-7")); err != nil || sBack.Int64() != -7 {
		t.Fatalf("SignedMicros.Scan([]byte) = %v, %v", sBack, err)
	}
}

func TestSQLScanRejects(t *testing.T) {
	t.Parallel()
	bad := []any{nil, int64(5), 1.5, "-1", "1.0", "", []byte("x")}
	for _, src := range bad {
		m := money.MicrosFromUint64(9)
		wantCode(t, m.Scan(src), errs.CodeDecodeFailed)
		if m.Uint64() != 9 {
			t.Fatalf("Micros.Scan(%#v) changed the value to %v", src, m)
		}
	}
	signedBad := []any{nil, int64(5), "-0", "1.0", []byte("--1")}
	for _, src := range signedBad {
		s := money.SignedMicrosFromInt64(9)
		wantCode(t, s.Scan(src), errs.CodeDecodeFailed)
		if s.Int64() != 9 {
			t.Fatalf("SignedMicros.Scan(%#v) changed the value to %v", src, s)
		}
	}
}
