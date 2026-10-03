package funding_test

import (
	"crypto/sha256"
	"encoding/base64"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
)

func TestOnrampStatus_allowsOnlyTheTransitionsTable(t *testing.T) {
	t.Parallel()
	allowed := map[domain.OnrampStatus][]domain.OnrampStatus{
		domain.OnrampCreated: {domain.OnrampOpened, domain.OnrampExpired},
		domain.OnrampOpened: {
			domain.OnrampConfirmed, domain.OnrampSubmitted, domain.OnrampCancelled, domain.OnrampFailed,
			domain.OnrampExpired,
		},
	}
	for _, from := range domain.OnrampStatuses() {
		for _, to := range domain.OnrampStatuses() {
			want := errs.CodeOnrampInvalidTransition
			if slices.Contains(allowed[from], to) {
				want = ""
			}
			if got := codeOrEmpty(from.To(to)); got != want {
				t.Errorf("%s -> %s = %q, want %q", from, to, got, want)
			}
		}
		if got, want := from.Terminal(), len(allowed[from]) == 0; got != want {
			t.Errorf("%s.Terminal() = %v, want %v", from, got, want)
		}
	}
}

func codeOrEmpty(err error) errs.Code {
	if err == nil {
		return ""
	}
	return errs.CodeOf(err)
}

func TestParseOnrampStatus_acceptsEveryStatusAndNothingElse(t *testing.T) {
	t.Parallel()
	for _, s := range domain.OnrampStatuses() {
		if got, err := domain.ParseOnrampStatus(string(s)); err != nil || got != s {
			t.Errorf("ParseOnrampStatus(%q) = %q, %v", s, got, err)
		}
	}
	if _, err := domain.ParseOnrampStatus("refunded"); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("ParseOnrampStatus(refunded) = %v, want decode_failed", err)
	}
}

func TestOnrampToken_roundTripsAndHashesWithSHA256(t *testing.T) {
	t.Parallel()
	var raw [domain.OnrampTokenBytes]byte
	for i := range raw {
		raw[i] = byte(i * 7)
	}
	token := domain.NewOnrampToken(raw)
	encoded := token.Encode()
	if strings.ContainsAny(encoded, "+/=") || len(encoded) != 43 {
		t.Fatalf("Encode() = %q, want 43 unpadded base64url characters", encoded)
	}
	parsed, err := domain.ParseOnrampToken(encoded)
	if err != nil || parsed != token {
		t.Fatalf("ParseOnrampToken(Encode()) = %v, %v", parsed, err)
	}
	if sum := sha256.Sum256(raw[:]); string(token.Hash()) != string(sum[:]) {
		t.Fatal("Hash() is not the SHA-256 of the raw token")
	}
	short := base64.RawURLEncoding.EncodeToString(raw[:31])
	for _, bad := range []string{"", "not base64!", short, encoded + "A"} {
		if _, err := domain.ParseOnrampToken(bad); errs.CodeOf(err) != errs.CodeOnrampLinkInvalid {
			t.Errorf("ParseOnrampToken(%q) = %v, want onramp_link_invalid", bad, err)
		}
	}
}

func TestRefuseOnrampToken_namesAnExpiredTokenAndCallsEveryOtherRefusalInvalid(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		status    domain.OnrampStatus
		wasOpened bool
		expiresAt time.Time
		want      errs.Code
	}{
		{"created past its expiry", domain.OnrampCreated, false, now.Add(-time.Second), errs.CodeOnrampLinkExpired},
		{"created at its expiry", domain.OnrampCreated, false, now, errs.CodeOnrampLinkExpired},
		{"expired by the poller before opening", domain.OnrampExpired, false, now, errs.CodeOnrampLinkExpired},
		{"expired after opening", domain.OnrampExpired, true, now, errs.CodeOnrampLinkInvalid},
		{"already opened", domain.OnrampOpened, true, now.Add(time.Minute), errs.CodeOnrampLinkInvalid},
		{"already confirmed", domain.OnrampConfirmed, true, now.Add(time.Minute), errs.CodeOnrampLinkInvalid},
	}
	for _, tt := range tests {
		if got := errs.CodeOf(domain.RefuseOnrampToken(tt.status, tt.wasOpened, tt.expiresAt, now)); got != tt.want {
			t.Errorf("%s: code = %s, want %s", tt.name, got, tt.want)
		}
	}
}
