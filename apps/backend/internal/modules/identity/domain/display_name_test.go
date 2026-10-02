package domain

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type displayNameFixture struct {
	Input      string `json:"input"`
	Normalized string `json:"normalized"`
	Reason     string `json:"reason"`
}

func TestParseDisplayNameSharedFixtures(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/display_names.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []displayNameFixture
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		got, err := ParseDisplayName(fixture.Input)
		if fixture.Reason != "" {
			if errs.CodeOf(err) != errs.CodeDisplayNameInvalid || reasonOf(err) != fixture.Reason {
				t.Fatalf("%q: %v", fixture.Input, err)
			}
			continue
		}
		if err != nil || got.String() != fixture.Normalized {
			t.Fatalf("%q: %q, %v", fixture.Input, got.String(), err)
		}
	}
}

func reasonOf(err error) string {
	for _, attr := range errs.Detail(err) {
		if attr.Key == "reason" {
			return attr.Value.String()
		}
	}
	return ""
}

func TestParseDisplayName(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ raw, want string }{
		{"  Kai   Q ", "Kai Q"}, {"Cafe\u0301", "Café"}, {"🦈 Kai", "🦈 Kai"},
	} {
		got, err := ParseDisplayName(tc.raw)
		if err != nil || got.String() != tc.want {
			t.Fatalf("ParseDisplayName(%q) = %q, %v", tc.raw, got.String(), err)
		}
	}
}

func TestParseDisplayNameRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ raw, reason string }{{"", "required"}, {"Kai\xff", "invalid_characters"}, {strings.Repeat("a", 33), "too_long"}} {
		_, err := ParseDisplayName(tc.raw)
		if errs.CodeOf(err) != errs.CodeDisplayNameInvalid || reasonOf(err) != tc.reason {
			t.Fatalf("ParseDisplayName(%q) = %v", tc.raw, err)
		}
	}
}
