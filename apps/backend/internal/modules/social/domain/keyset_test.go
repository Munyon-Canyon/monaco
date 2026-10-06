package domain_test

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
)

func TestKeyset_roundTripsAtMicrosecondPrecision(t *testing.T) {
	t.Parallel()
	want := domain.Keyset{
		At: time.Date(2026, 10, 3, 8, 15, 30, 123_456_000, time.UTC),
		ID: uuid.UUID{0x01, 0x92, 0x00, 0x00, 0x00, 0x00, 0x70, 0x00, 0x80, 0x00, 0, 0, 0, 0, 0, 7},
	}
	got, err := domain.ParseKeyset(want.Encode())
	if err != nil {
		t.Fatal(err)
	}
	if !got.At.Equal(want.At) || got.ID != want.ID {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

func TestParseKeyset_refusesAnythingItDidNotWrite(t *testing.T) {
	t.Parallel()
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	for _, raw := range []string{
		"!!!",
		enc("no-separator"),
		enc("soon.01920000-0000-7000-8000-000000000007"),
		enc("1759479330123456.not-a-uuid"),
	} {
		if _, err := domain.ParseKeyset(raw); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("ParseKeyset(%q) err = %v, want invalid_input", raw, err)
		}
	}
}

func TestRankedKeyset_roundTripsTheCountAndStaysApartFromAPlainCursor(t *testing.T) {
	t.Parallel()
	want := domain.Keyset{
		Ranked: true, Count: 12, At: time.Date(2026, 10, 3, 8, 15, 30, 123_456_000, time.UTC),
		ID: uuid.UUID{0x01, 0x92, 0x00, 0x00, 0x00, 0x00, 0x70, 0x00, 0x80, 0x00, 0, 0, 0, 0, 0, 7},
	}
	got, err := domain.ParseRankedKeyset(want.Encode())
	if err != nil || !got.Ranked || got.Count != 12 || !got.At.Equal(want.At) || got.ID != want.ID {
		t.Fatalf("round trip = %+v, %v, want %+v", got, err, want)
	}
	if _, err := domain.ParseKeyset(want.Encode()); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Errorf("a ranked cursor read as a plain one: err = %v, want invalid_input", err)
	}
	plain := domain.Keyset{At: want.At, ID: want.ID}
	if _, err := domain.ParseRankedKeyset(plain.Encode()); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Errorf("a plain cursor read as a ranked one: err = %v, want invalid_input", err)
	}
}

func TestParseRankedKeyset_refusesAnythingItDidNotWrite(t *testing.T) {
	t.Parallel()
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	for _, raw := range []string{
		"!!!",
		enc("no-separator"),
		enc("many.1759479330123456.01920000-0000-7000-8000-000000000007"),
		enc("3.soon.01920000-0000-7000-8000-000000000007"),
		enc("3.1759479330123456.not-a-uuid"),
		enc("3.1759479330123456"),
	} {
		if _, err := domain.ParseRankedKeyset(raw); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("ParseRankedKeyset(%q) err = %v, want invalid_input", raw, err)
		}
	}
}
