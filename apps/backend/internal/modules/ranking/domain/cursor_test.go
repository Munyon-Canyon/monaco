package domain_test

import (
	"encoding/base64"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
)

func TestCursor_roundTrip(t *testing.T) {
	t.Parallel()
	for _, rank := range []int{0, 1, 20, 4500} {
		got, err := domain.DecodeCursor(domain.EncodeCursor(rank))
		if err != nil || got != rank {
			t.Errorf("DecodeCursor(EncodeCursor(%d)) = %d, %v", rank, got, err)
		}
	}
}

func TestCursor_rejectsTamperedTokens(t *testing.T) {
	t.Parallel()
	negative := base64.RawURLEncoding.EncodeToString([]byte("-3"))
	text := base64.RawURLEncoding.EncodeToString([]byte("abc"))
	for _, raw := range []string{"", "!!!", negative, text} {
		if _, err := domain.DecodeCursor(raw); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("DecodeCursor(%q) err = %v, want invalid_input", raw, err)
		}
	}
}

func TestParseLimit_bounds(t *testing.T) {
	t.Parallel()
	ptr := func(n int) *int { return &n }
	if got, err := domain.ParseLimit(nil); err != nil || got != domain.DefaultLimit {
		t.Fatalf("ParseLimit(nil) = %d, %v", got, err)
	}
	for _, n := range []int{1, 50} {
		if got, err := domain.ParseLimit(ptr(n)); err != nil || got != n {
			t.Errorf("ParseLimit(%d) = %d, %v", n, got, err)
		}
	}
	for _, n := range []int{-1, 0, 51} {
		if _, err := domain.ParseLimit(ptr(n)); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("ParseLimit(%d) err = %v, want invalid_input", n, err)
		}
	}
}
