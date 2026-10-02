package referrals_test

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/domain"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestCodeAlphabet_isTheUnambiguousLowercaseLiteral(t *testing.T) {
	t.Parallel()
	if domain.CodeAlphabet != "23456789abcdefghjkmnpqrstuvwxyz" {
		t.Fatalf("CodeAlphabet = %q; the referral_codes check pins this literal", domain.CodeAlphabet)
	}
}

func TestNewRandomCode_tenThousandCodesPassTheReferralCodesCheckConstraint(t *testing.T) {
	t.Parallel()
	codes := make([]string, 10_000)
	for i := range codes {
		code, err := domain.NewRandomCode(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		if !domain.IsRandomShape(string(code)) {
			t.Fatalf("NewRandomCode() = %q, which IsRandomShape rejects", code)
		}
		codes[i] = string(code)
	}
	pool := testkit.DB(t)
	_, err := pool.Exec(t.Context(), `
		INSERT INTO referral_codes (code, user_id, created_at)
		SELECT c, gen_random_uuid(), now() FROM unnest($1::text[]) AS c
		ON CONFLICT (code) DO NOTHING`, codes)
	if err != nil {
		t.Fatalf("inserting 10,000 generated codes: %v", err)
	}
}

func TestNewRandomCode_rejectsBytesThatWouldBiasTheAlphabet(t *testing.T) {
	t.Parallel()
	for name, tt := range map[string]struct {
		in   []byte
		want domain.Code
	}{
		"the first eight characters": {[]byte{0, 1, 2, 3, 4, 5, 6, 7}, "23456789"},
		"wraps at the alphabet size": {[]byte{31, 32, 33, 34, 35, 36, 37, 30}, "2345678z"},
		"the highest accepted byte":  {bytes.Repeat([]byte{247}, 8), "zzzzzzzz"},
		"skips 248 to 255 and reads more": {
			append([]byte{248, 255, 0, 1, 2, 3, 4, 5}, []byte{6, 7, 9, 9, 9, 9, 9, 9}...),
			"23456789",
		},
	} {
		got, err := domain.NewRandomCode(bytes.NewReader(tt.in))
		if err != nil || got != tt.want {
			t.Errorf("%s: NewRandomCode(%v) = %q, %v; want %q", name, tt.in, got, err, tt.want)
		}
	}
}

func TestNewRandomCode_failsInternalWhenTheSourceFails(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test.entropy")
	for name, tt := range map[string]struct {
		source io.Reader
		cause  error
	}{
		"an empty source":                 {strings.NewReader(""), io.EOF},
		"a short source":                  {strings.NewReader("abc"), io.ErrUnexpectedEOF},
		"a source of only rejected bytes": {bytes.NewReader(bytes.Repeat([]byte{255}, 8)), io.EOF},
		"a failing source":                {iotest.ErrReader(boom), boom},
	} {
		got, err := domain.NewRandomCode(tt.source)
		if errs.CodeOf(err) != errs.CodeInternal || !errors.Is(err, tt.cause) || got != "" {
			t.Errorf("%s: NewRandomCode() = %q, %v; want empty and internal wrapping %v", name, got, err, tt.cause)
		}
	}
}

func TestNormalizeInput_trimsAndLowercases(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"K7M4QX2P":       "k7m4qx2p",
		"  k7m4qx2p\t\n": "k7m4qx2p",
		" KaiCenat ":     "kaicenat",
		"":               "",
	} {
		if got := domain.NormalizeInput(in); got != want {
			t.Errorf("NormalizeInput(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsRandomShape_acceptsOnlyEightAlphabetCharacters(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]bool{
		"k7m4qx2p":  true,
		"23456789":  true,
		"zzzzzzzz":  true,
		"k7m4qx2":   false,
		"k7m4qx2pp": false,
		"K7M4QX2P":  false,
		"k7m4qx20":  false,
		"k7m4qx21":  false,
		"k7m4qxop":  false,
		"k7m4qxip":  false,
		"k7m4qxlp":  false,
		"k7m4 x2p":  false,
		"":          false,
	} {
		if got := domain.IsRandomShape(in); got != want {
			t.Errorf("IsRandomShape(%q) = %v, want %v", in, got, want)
		}
	}
}
