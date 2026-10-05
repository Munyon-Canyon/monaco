package domain_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
)

func TestParseMentions(t *testing.T) {
	t.Parallel()
	many := make([]string, 0, 25)
	for i := range 25 {
		many = append(many, fmt.Sprintf("@user%02d", i))
	}
	first20 := make([]string, 0, domain.MaxMentions)
	for _, m := range many[:domain.MaxMentions] {
		first20 = append(first20, m[1:])
	}
	tests := []struct {
		name string
		body string
		want []string
	}{
		{"start of line", "@alice hi", []string{"alice"}},
		{"after punctuation", "hey,@bob! (@carol) @dan.", []string{"bob", "carol", "dan"}},
		{"after newline", "hi\n@erin", []string{"erin"}},
		{"email", "mail a@b.com or bob@example.com", nil},
		{"url", "see https://x.com/@alice and http://y.io/p?u=@bob", nil},
		{"url then mention", "https://x.com/@alice @carol", []string{"carol"}},
		{"double at", "@@xyz", nil},
		{"uppercase folded", "@Alice_99 @ALICE_99", []string{"alice_99"}},
		{"21-character handle", "@" + strings.Repeat("a", 21), nil},
		{"20-character handle", "@" + strings.Repeat("a", 20), []string{strings.Repeat("a", 20)}},
		{"2-character handle", "@ab", nil},
		{"duplicates", "@bob @alice @bob", []string{"bob", "alice"}},
		{"25 unique mentions", strings.Join(many, " "), first20},
		{"empty body", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.ParseMentions(tt.body); !slices.Equal(got, tt.want) {
				t.Fatalf("ParseMentions(%q) = %v, want %v", tt.body, got, tt.want)
			}
		})
	}
}
