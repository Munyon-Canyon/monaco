package domain_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
)

func TestParseClientSource(t *testing.T) {
	t.Parallel()
	accepted := map[string]domain.FollowSource{
		"":          domain.SourceProfile,
		"profile":   domain.SourceProfile,
		"phone":     domain.SourcePhone,
		"x":         domain.SourceX,
		"cabal":     domain.SourceCabal,
		"feed":      domain.SourceFeed,
		"suggested": domain.SourceSuggested,
	}
	for raw, want := range accepted {
		got, err := domain.ParseClientSource(raw)
		if err != nil || got != want || got.String() != string(want) {
			t.Errorf("ParseClientSource(%q) = %q, %v, want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{"referral", "Profile", "twitter", " "} {
		got, err := domain.ParseClientSource(raw)
		if code := errs.CodeOf(err); err == nil || code != errs.CodeInvalidInput || got != "" {
			t.Errorf("ParseClientSource(%q) = %q, %v (code %s), want invalid_input", raw, got, err, code)
		}
	}
}
