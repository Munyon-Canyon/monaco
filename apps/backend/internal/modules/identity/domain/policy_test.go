package domain

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func TestReferralAlphabetIsThePinnedLiteral(t *testing.T) {
	t.Parallel()
	const pinned = "23456789abcdefghjkmnpqrstuvwxyz"
	if ReferralAlphabet != pinned {
		t.Fatalf("ReferralAlphabet = %q, want %q", ReferralAlphabet, pinned)
	}
}

func TestReservedListIsTheHandleBlocklist(t *testing.T) {
	t.Parallel()
	want := []string{
		"admin", "monaco", "monacolabs", "support", "help", "api", "app", "r", "official", "team",
		"root", "system", "staff", "mod", "moderator", "security", "billing", "cabal", "cabals",
		"null", "undefined", "me", "settings",
	}
	if len(reservedHandles()) != len(want) {
		t.Fatalf("reserved has %d names, want %d", len(reservedHandles()), len(want))
	}
	for _, name := range want {
		if !reservedHandle(name) {
			t.Errorf("reserved list missing %q", name)
		}
	}
}

func TestProfanityListIsShortEnglishTerms(t *testing.T) {
	t.Parallel()
	terms := loadProfanity(profanityFile)
	if len(terms) == 0 || len(terms) > 200 {
		t.Fatalf("profanity list has %d terms, want 1 to 200", len(terms))
	}
	for _, term := range terms {
		if strings.Trim(term, "abcdefghijklmnopqrstuvwxyz") != "" {
			t.Errorf("profanity term %q is not lowercase english", term)
		}
	}
}

func TestFoldLeetMapsEachDigit(t *testing.T) {
	t.Parallel()
	if got := foldLeet("013457x"); got != "oieastx" {
		t.Fatalf("foldLeet = %q, want oieastx", got)
	}
	if got := foldLeet("abc_"); got != "abc_" {
		t.Fatalf("foldLeet = %q, want abc_", got)
	}
}

func TestHandleSetRequiresAHandle(t *testing.T) {
	t.Parallel()
	if err := HandleSet(User{}); errs.CodeOf(err) != errs.CodeHandleRequired {
		t.Fatalf("HandleSet(empty) = %v, want %s", err, errs.CodeHandleRequired)
	}
	if err := HandleSet(User{Handle: "kai"}); err != nil {
		t.Fatalf("HandleSet(kai) = %v, want nil", err)
	}
}

func TestHandlePolicyCheck(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 2, 15, 0, 0, 0, time.UTC)
	fresh := now.Add(-time.Minute)
	ready := now.Add(-HandleChangeInterval)
	cases := []struct {
		name  string
		raw   string
		facts HandleFacts
		want  errs.Code
	}{
		{name: "plain", raw: "kai_1", want: ""},
		{name: "reserved", raw: "admin", want: errs.CodeHandleReserved},
		{name: "reserved is exact", raw: "adminx", want: ""},
		{name: "profanity", raw: "xxfuckxx", want: errs.CodeHandleReserved},
		{name: "leet profanity", raw: "sh1t", want: errs.CodeHandleReserved},
		{name: "ordinary therapist", raw: "therapist", want: ""},
		{name: "ordinary cocktail", raw: "cocktail", want: ""},
		{name: "ordinary peacock", raw: "peacock", want: ""},
		{name: "ordinary smartwatch", raw: "smartwatch", want: ""},
		{name: "ordinary swanky", raw: "swanky", want: ""},
		{name: "ordinary dickens", raw: "dickens", want: ""},
		{name: "ordinary prickly", raw: "prickly", want: ""},
		{name: "ordinary ashkenazi", raw: "ashkenazi", want: ""},
		{name: "code shape", raw: "23456789", want: errs.CodeHandleReserved},
		{name: "code shape letters", raw: "abcdefgh", want: errs.CodeHandleReserved},
		{name: "code shape miss", raw: "abcdefg1", want: ""},
		{name: "short of code shape", raw: "abcdefg", want: ""},
		{
			name:  "other x username",
			raw:   "kaicenat",
			facts: HandleFacts{OtherUserXUsernameMatch: true},
			want:  errs.CodeHandleTaken,
		},
		{name: "own x skips reserved", raw: "admin", facts: HandleFacts{OwnXUsername: "AdMiN"}, want: ""},
		{
			name: "own x skips profanity and impersonation",
			raw:  "xxfuckxx",
			facts: HandleFacts{
				OwnXUsername: "xxFUCkxx", OtherUserXUsernameMatch: true,
			},
			want: "",
		},
		{
			name:  "own x still hits the change limit",
			raw:   "admin",
			facts: HandleFacts{OwnXUsername: "admin", CurrentHandle: "kai", HandleChangedAt: fresh, Now: now},
			want:  errs.CodeHandleTooSoon,
		},
		{
			name:  "too soon",
			raw:   "next_one",
			facts: HandleFacts{CurrentHandle: "kai", HandleChangedAt: fresh, Now: now},
			want:  errs.CodeHandleTooSoon,
		},
		{
			name:  "exactly thirty days is allowed",
			raw:   "next_one",
			facts: HandleFacts{CurrentHandle: "kai", HandleChangedAt: ready, Now: now},
			want:  "",
		},
		{
			name:  "same handle skips the change limit",
			raw:   "kai",
			facts: HandleFacts{CurrentHandle: "kai", HandleChangedAt: fresh, Now: now},
			want:  "",
		},
		{
			name:  "first handle has no change limit",
			raw:   "kai",
			facts: HandleFacts{HandleChangedAt: fresh, Now: now},
			want:  "",
		},
		{
			name: "reserved wins over taken and too soon",
			raw:  "monaco",
			facts: HandleFacts{
				OtherUserXUsernameMatch: true, CurrentHandle: "kai", HandleChangedAt: fresh, Now: now,
			},
			want: errs.CodeHandleReserved,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, err := ParseHandle(tc.raw)
			if err != nil {
				t.Fatalf("ParseHandle(%q) = %v", tc.raw, err)
			}
			err = HandlePolicy{}.Check(h, tc.facts)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Check = %v, want nil", err)
				}
				return
			}
			if errs.CodeOf(err) != tc.want {
				t.Fatalf("Check = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestProperty_ParseHandleAcceptsExactlyTheHandlePattern(t *testing.T) {
	t.Parallel()
	pattern := regexp.MustCompile(`^[a-z0-9_]{3,20}$`)
	rapid.Check(t, func(t *rapid.T) {
		raw := rapid.OneOf(
			rapid.StringMatching(`^[A-Za-z0-9_]{3,20}$`),
			rapid.StringN(0, 2, 8),
			rapid.StringMatching(`^[A-Za-z0-9_]{21,30}$`),
			rapid.StringN(0, 24, 48),
			rapid.Custom(func(t *rapid.T) string {
				base := rapid.StringMatching(`^[A-Za-z0-9_]{0,12}$`).Draw(t, "base")
				bad := rapid.RuneFrom([]rune(" .-@/ï")).Draw(t, "bad")
				return base + string(bad)
			}),
		).Draw(t, "raw")
		folded := strings.ToLower(raw)
		got, err := ParseHandle(raw)
		if pattern.MatchString(folded) {
			if err != nil || got.String() != folded {
				t.Fatalf("ParseHandle(%q) = %q, %v, want %q", raw, got, err, folded)
			}
			return
		}
		if err == nil || errs.CodeOf(err) != errs.CodeHandleInvalid || got.String() != "" {
			t.Fatalf("ParseHandle(%q) = %q, %v, want %s", raw, got, err, errs.CodeHandleInvalid)
		}
	})
}

func TestParseHandle_foldsCaseAndUnicodeThatLowersIntoThePattern(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]string{
		"KaiCenat":              "kaicenat",
		"abc":                   "abc",
		"a_1":                   "a_1",
		strings.Repeat("Z", 20): strings.Repeat("z", 20),
		"Kai":                   "kai",
		"İii":                   "iii",
	} {
		got, err := ParseHandle(raw)
		if err != nil || got.String() != want {
			t.Errorf("ParseHandle(%q) = %q, %v, want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{
		"", "ab", strings.Repeat("z", 21), "kai.cenat", "kai-cenat", "kai cenat", "kaï", "@kai",
	} {
		got, err := ParseHandle(raw)
		if err == nil || errs.CodeOf(err) != errs.CodeHandleInvalid || got.String() != "" {
			t.Errorf("ParseHandle(%q) = %q, %v, want %s", raw, got, err, errs.CodeHandleInvalid)
		}
	}
}
