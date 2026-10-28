package domain_test

import (
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
)

func TestParseEmoji(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"thumbs up", "👍", "👍"},
		{"thumbs up with a skin tone", "👍🏽", "👍🏽"},
		{"red heart", "❤\uFE0F", "❤\uFE0F"},
		{"bare heart folds to the red heart", "❤", "❤\uFE0F"},
		{"flag", "🇺🇸", "🇺🇸"},
		{
			"subdivision flag",
			"🏴\U000E0067\U000E0062\U000E0073\U000E0063\U000E0074\U000E007F",
			"🏴\U000E0067\U000E0062\U000E0073\U000E0063\U000E0074\U000E007F",
		},
		{"keycap", "1\uFE0F\u20E3", "1\uFE0F\u20E3"},
		{"family", "👨\u200D👩\u200D👧", "👨\u200D👩\u200D👧"},
		{"heart on fire", "❤\uFE0F\u200D🔥", "❤\uFE0F\u200D🔥"},
		{"rainbow flag", "🏳\uFE0F\u200D🌈", "🏳\uFE0F\u200D🌈"},
		{"technologist with a skin tone", "🧑🏽\u200D💻", "🧑🏽\u200D💻"},
		{"letter", "a", ""},
		{"two emoji", "👍👍", ""},
		{"trailing space", "👍 ", ""},
		{"empty", "", ""},
		{"zero width joiner alone", "\u200D", ""},
		{"40 bytes", strings.Repeat("👍", 10), ""},
		{"digit without the keycap", "1", ""},
		{"lone regional indicator", "🇺", ""},
		{"three regional indicators", "🇺🇸🇺", ""},
		{"trailing joiner", "👍\u200D", ""},
		{"skin tone alone", "🏽", ""},
		{"variation selector alone", "\uFE0F", ""},
		{"flag joined to a pictograph", "🇺🇸\u200D👍", ""},
		{"invalid UTF-8", "👍\xff", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := domain.ParseEmoji(tt.raw)
			if tt.want == "" {
				if errs.CodeOf(err) != errs.CodeInvalidInput ||
					!slices.ContainsFunc(errs.Detail(err), slog.String("field", "emoji").Equal) {
					t.Fatalf("ParseEmoji(%q) = %q, %v, want invalid_input on field emoji", tt.raw, got, err)
				}
				return
			}
			if err != nil || got.String() != tt.want {
				t.Fatalf("ParseEmoji(%q) = %q, %v, want %q", tt.raw, got, err, tt.want)
			}
		})
	}
}

func TestParseEmoji_bareAndRedHeartAreOneReaction(t *testing.T) {
	t.Parallel()
	bare, err := domain.ParseEmoji("❤")
	if err != nil {
		t.Fatal(err)
	}
	red, err := domain.ParseEmoji("❤\uFE0F")
	if err != nil || bare != red {
		t.Fatalf("bare %q and red %q, %v, want one emoji", bare, red, err)
	}
}
