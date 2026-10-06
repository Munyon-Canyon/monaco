package domain_test

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
)

func TestParsePhoneHashes_acceptsLowercaseHexAndCollapsesDuplicates(t *testing.T) {
	t.Parallel()
	sum := sha256.Sum256([]byte("+14155550123"))
	want := hex.EncodeToString(sum[:])
	crafted := strings.Repeat("0123456789abcdef", 4)
	got, err := domain.ParsePhoneHashes([]string{want, want, crafted})
	if err != nil || len(got) != 2 || hex.EncodeToString(got[0]) != want || hex.EncodeToString(got[1]) != crafted {
		t.Fatalf("ParsePhoneHashes = %x, %v, want the phone hash once and the crafted hash", got, err)
	}
}

func TestParsePhoneHashes_refusesEmptyUppercaseShortAndTooMany(t *testing.T) {
	t.Parallel()
	sum := sha256.Sum256([]byte("+14155550123"))
	valid := hex.EncodeToString(sum[:])
	tooMany := make([]string, domain.MaxContactHashes+1)
	for i := range tooMany {
		tooMany[i] = valid
	}
	tests := []struct {
		name string
		raw  []string
		want errs.Code
	}{
		{"empty", nil, errs.CodeContactHashesInvalid},
		{"empty list", []string{}, errs.CodeContactHashesInvalid},
		{"uppercase", []string{strings.ToUpper(valid)}, errs.CodeContactHashesInvalid},
		{"short", []string{valid[:63]}, errs.CodeContactHashesInvalid},
		{"too many", tooMany, errs.CodeTooManyContactHashes},
	}
	for _, tt := range tests {
		_, err := domain.ParsePhoneHashes(tt.raw)
		if got := errs.CodeOf(err); err == nil || got != tt.want {
			t.Errorf("%s: err = %v (code %s), want %s", tt.name, err, got, tt.want)
		}
	}
	if _, err := domain.ParsePhoneHashes([]string{valid}); err != nil {
		t.Fatal(err)
	}
	full := make([]string, domain.MaxContactHashes)
	for i := range full {
		full[i] = valid
	}
	got, err := domain.ParsePhoneHashes(full)
	if err != nil || len(got) != 1 {
		t.Fatalf("2000 copies = %d hashes, %v, want one", len(got), err)
	}
}
