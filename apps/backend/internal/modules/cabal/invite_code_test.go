package cabal_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
)

func TestNewInviteCode_mapsEachByteToTheCrockfordCharacterOfItsLowFiveBits(t *testing.T) {
	t.Parallel()
	for name, tt := range map[string]struct {
		in   []byte
		want string
	}{
		"the first ten values":  {[]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, "0123456789"},
		"the last ten values":   {[]byte{22, 23, 24, 25, 26, 27, 28, 29, 30, 31}, "PQRSTVWXYZ"},
		"high bits are ignored": {[]byte{32, 33, 34, 35, 36, 37, 38, 39, 40, 41}, "0123456789"},
		"all ones":              {bytes.Repeat([]byte{0xff}, 10), "ZZZZZZZZZZ"},
		"skips I, L, O and U":   {[]byte{17, 18, 19, 20, 21, 22, 26, 27, 0, 31}, "HJKMNPTV0Z"},
	} {
		got, err := domain.NewInviteCode(bytes.NewReader(tt.in))
		if err != nil || got.String() != tt.want {
			t.Errorf("%s: NewInviteCode(%v) = %q, %v; want %q", name, tt.in, got, err, tt.want)
		}
	}
}

func TestNewInviteCode_readsExactlyTenBytesAndFailsInternalWhenTheSourceFails(t *testing.T) {
	t.Parallel()
	source := bytes.NewReader(bytes.Repeat([]byte{7}, 15))
	if _, err := domain.NewInviteCode(source); err != nil || source.Len() != 5 {
		t.Fatalf("NewInviteCode read %d of 15 bytes, %v; want exactly 10", 15-source.Len(), err)
	}
	boom := errs.New(errs.CodeInternal, "test.entropy")
	for name, tt := range map[string]struct {
		source io.Reader
		cause  error
	}{
		"a short source":   {strings.NewReader("abc"), io.ErrUnexpectedEOF},
		"an empty source":  {strings.NewReader(""), io.EOF},
		"a failing source": {iotest.ErrReader(boom), boom},
	} {
		got, err := domain.NewInviteCode(tt.source)
		wantCode(t, name, err, errs.CodeInternal)
		if !errors.Is(err, tt.cause) || got.String() != "" {
			t.Errorf("%s: err = %v and code %q; want it to wrap %v and be empty", name, err, got, tt.cause)
		}
	}
}

func TestParseInviteCode_normalizesCaseAndCrockfordLookalikes(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ raw, want string }{
		{"ABCDEFGHJK", "ABCDEFGHJK"},
		{"abcdefghjk", "ABCDEFGHJK"},
		{"  0123456789 ", "0123456789"},
		{"OILOILOILO", "0110110110"},
		{"oiloiloilo", "0110110110"},
		{"ZYXWVTSRQP", "ZYXWVTSRQP"},
	} {
		got, err := domain.ParseInviteCode(tt.raw)
		if err != nil || got.String() != tt.want {
			t.Errorf("ParseInviteCode(%q) = %q, %v; want %q", tt.raw, got, err, tt.want)
		}
	}
}

func TestParseInviteCode_refusesTheWrongLengthAlphabetOrCharacters(t *testing.T) {
	t.Parallel()
	for name, raw := range map[string]string{
		"empty":         "",
		"nine":          "ABCDEFGHJ",
		"eleven":        "ABCDEFGHJKM",
		"contains U":    "ABCDEFGHJU",
		"hyphenated":    "ABCDE-FGHJ",
		"inner space":   "ABCDE FGHJ",
		"non ascii":     "ABCDEFGHJé",
		"twenty digits": strings.Repeat("1", 20),
	} {
		got, err := domain.ParseInviteCode(raw)
		wantCode(t, name, err, errs.CodeInvalidInput)
		if got.String() != "" {
			t.Errorf("%s: code = %q, want the zero value", name, got)
		}
	}
}

func lowFiveBits(raw []byte) []byte {
	out := make([]byte, len(raw))
	for i, b := range raw {
		out[i] = b & 0x1f
	}
	return out
}

func TestNewInviteCode_isTenCrockfordCharactersOfTheFiftyLowBitsAndParsesBack(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		a := rapid.SliceOfN(rapid.Byte(), 10, 10).Draw(t, "a")
		b := rapid.SliceOfN(rapid.Byte(), 10, 10).Draw(t, "b")
		codeA, errA := domain.NewInviteCode(bytes.NewReader(a))
		codeB, errB := domain.NewInviteCode(bytes.NewReader(b))
		masked, errM := domain.NewInviteCode(bytes.NewReader(lowFiveBits(a)))
		if errA != nil || errB != nil || errM != nil {
			t.Fatalf("NewInviteCode failed: %v, %v, %v", errA, errB, errM)
		}
		if len(codeA.String()) != 10 {
			t.Fatalf("code %q is not 10 characters", codeA)
		}
		if _, err := domain.ParseInviteCode(codeA.String()); err != nil {
			t.Fatalf("code %q is not an invite code: %v", codeA, err)
		}
		if masked != codeA {
			t.Fatalf("code of %v = %q but of its low bits %q; only the low five bits may matter", a, codeA, masked)
		}
		if same := bytes.Equal(lowFiveBits(a), lowFiveBits(b)); same != (codeA == codeB) {
			t.Fatalf("codes %q and %q for %v and %v: equal = %t, want %t", codeA, codeB, a, b, codeA == codeB, same)
		}
		if parsed, err := domain.ParseInviteCode(codeA.String()); err != nil || parsed != codeA {
			t.Fatalf("ParseInviteCode(%q) = %q, %v; want the same code", codeA, parsed, err)
		}
	})
}
