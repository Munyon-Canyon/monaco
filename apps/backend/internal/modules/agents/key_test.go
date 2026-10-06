package agents_test

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/agents/domain"
)

const zeroKey = "monaco_ak_22222222222222222222222222222222"

var keyShape = regexp.MustCompile(`^monaco_ak_[23456789abcdefghjkmnpqrstuvwxyz]{32}$`)

func mintZeroKey(t *testing.T) domain.Key {
	t.Helper()
	key, err := domain.MintKey(bytes.NewReader(make([]byte, domain.KeyLength)))
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestMintKey_Format(t *testing.T) {
	t.Parallel()
	random, err := domain.MintKey(rand.Reader)
	if err != nil || !keyShape.MatchString(random.String()) {
		t.Fatalf("MintKey(crypto/rand) = %q, %v; want monaco_ak_ and 32 alphabet characters", random.String(), err)
	}
	zeros, err := domain.MintKey(bytes.NewReader(make([]byte, 64)))
	if err != nil || zeros.String() != zeroKey {
		t.Fatalf("MintKey(64 zero bytes) = %q, %v; want %q", zeros.String(), err, zeroKey)
	}
	boom := errs.New(errs.CodeInternal, "test.entropy")
	for name, tt := range map[string]struct {
		source io.Reader
		cause  error
	}{
		"an empty source":                 {strings.NewReader(""), io.EOF},
		"a short source":                  {strings.NewReader("abc"), io.ErrUnexpectedEOF},
		"a source of only rejected bytes": {bytes.NewReader(bytes.Repeat([]byte{255}, 32)), io.EOF},
		"a failing source":                {iotest.ErrReader(boom), boom},
	} {
		got, err := domain.MintKey(tt.source)
		var e *errs.Error
		if !errors.As(err, &e) || e.Code != errs.CodeInternal || e.Op != "agents.MintKey" ||
			!errors.Is(err, tt.cause) || got.String() != "" {
			t.Errorf("%s: MintKey() = %q, %v; want an empty key and internal from agents.MintKey wrapping %v",
				name, got.String(), err, tt.cause)
		}
	}
}

func TestMintKey_rejectsBytesThatWouldBiasTheAlphabet(t *testing.T) {
	t.Parallel()
	ascending := make([]byte, domain.KeyLength)
	for i := range ascending {
		ascending[i] = byte(i)
	}
	for name, tt := range map[string]struct {
		in   []byte
		want string
	}{
		"wraps at the alphabet size": {ascending, domain.KeyPrefix + domain.KeyAlphabet + "2"},
		"the highest accepted byte":  {bytes.Repeat([]byte{247}, 32), domain.KeyPrefix + strings.Repeat("z", 32)},
		"skips 248 to 255 and reads on": {
			append([]byte{248, 249, 250, 251, 252, 253, 254, 255}, bytes.Repeat([]byte{1}, 56)...),
			domain.KeyPrefix + strings.Repeat("3", 32),
		},
	} {
		got, err := domain.MintKey(bytes.NewReader(tt.in))
		if err != nil || got.String() != tt.want {
			t.Errorf("%s: MintKey(%v) = %q, %v; want %q", name, tt.in, got.String(), err, tt.want)
		}
	}
}

func TestParseKey_acceptsOnlyThePrefixAndThirtyTwoAlphabetCharacters(t *testing.T) {
	t.Parallel()
	body := strings.Repeat("k", domain.KeyLength)
	valid := domain.KeyPrefix + body
	if got, err := domain.ParseKey(valid); err != nil || got.String() != valid {
		t.Fatalf("ParseKey(%q) = %q, %v", valid, got.String(), err)
	}
	for name, raw := range map[string]string{
		"an empty string":      "",
		"no prefix":            body,
		"a wrong prefix":       "monaco_ak-" + body,
		"31 characters":        valid[:len(valid)-1],
		"33 characters":        valid + "k",
		"a trailing newline":   valid + "\n",
		"a zero first":         domain.KeyPrefix + "0" + body[1:],
		"a zero in the middle": domain.KeyPrefix + body[:15] + "0" + body[16:],
		"a zero last":          domain.KeyPrefix + body[:31] + "0",
		"a one":                domain.KeyPrefix + body[:31] + "1",
		"an l":                 domain.KeyPrefix + body[:31] + "l",
		"an o":                 domain.KeyPrefix + body[:31] + "o",
		"an uppercase letter":  domain.KeyPrefix + body[:31] + "K",
	} {
		got, err := domain.ParseKey(raw)
		var e *errs.Error
		if got.String() != "" || !errors.As(err, &e) || e.Code != errs.CodeInvalidInput || e.Op != "agents.ParseKey" {
			t.Errorf("%s: ParseKey(%q) = %q, %v; want an empty key and invalid_input from agents.ParseKey",
				name, raw, got.String(), err)
			continue
		}
		if raw == "" {
			continue
		}
		for _, attr := range errs.Detail(err) {
			if strings.Contains(attr.String(), raw) {
				t.Errorf("%s: attribute %s carries the raw value", name, attr)
			}
		}
		if strings.Contains(err.Error(), raw) {
			t.Errorf("%s: error %q carries the raw value", name, err)
		}
	}
}

func TestKey_logsAsRedactedAndNeverAsThePlaintext(t *testing.T) {
	t.Parallel()
	key := mintZeroKey(t)
	var out bytes.Buffer
	slog.New(slog.NewJSONHandler(&out, nil)).InfoContext(t.Context(), "minted", slog.Any("key", key))
	if got := out.String(); !strings.Contains(got, "[redacted]") || strings.Contains(got, strings.Repeat("2", 32)) {
		t.Fatalf("log line %q, want [redacted] and no plaintext", got)
	}
}

func TestKey_hashIsTheSHA256OfThePlaintext(t *testing.T) {
	t.Parallel()
	const want = "07e9de9a3c761d49a9b1aaf18dad8c215412038c2cc89badf587af246096f00a"
	if got := hex.EncodeToString(mintZeroKey(t).Hash()); got != want {
		t.Fatalf("Hash() = %s, want %s", got, want)
	}
}
