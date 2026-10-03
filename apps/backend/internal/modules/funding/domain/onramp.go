package domain

import (
	"crypto/sha256"
	"encoding/base64"
	"log/slog"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type OnrampStatus string

const (
	OnrampCreated   OnrampStatus = "created"
	OnrampOpened    OnrampStatus = "opened"
	OnrampConfirmed OnrampStatus = "confirmed"
	OnrampSubmitted OnrampStatus = "submitted"
	OnrampCancelled OnrampStatus = "cancelled"
	OnrampFailed    OnrampStatus = "failed"
	OnrampExpired   OnrampStatus = "expired"
)

func OnrampStatuses() []OnrampStatus {
	return []OnrampStatus{
		OnrampCreated, OnrampOpened, OnrampConfirmed, OnrampSubmitted, OnrampCancelled, OnrampFailed, OnrampExpired,
	}
}

func onrampTransitions() map[OnrampStatus][]OnrampStatus {
	return map[OnrampStatus][]OnrampStatus{
		OnrampCreated: {OnrampOpened, OnrampExpired},
		OnrampOpened:  {OnrampConfirmed, OnrampSubmitted, OnrampCancelled, OnrampFailed, OnrampExpired},
	}
}

func ParseOnrampStatus(raw string) (OnrampStatus, error) {
	if !slices.Contains(OnrampStatuses(), OnrampStatus(raw)) {
		return "", errs.New(errs.CodeDecodeFailed, "funding.ParseOnrampStatus", slog.String("raw", raw))
	}
	return OnrampStatus(raw), nil
}

func (s OnrampStatus) To(to OnrampStatus) error {
	if !slices.Contains(onrampTransitions()[s], to) {
		return errs.New(errs.CodeOnrampInvalidTransition, "funding.OnrampStatus.To",
			slog.String("from", string(s)), slog.String("to", string(to)))
	}
	return nil
}

func ReportedOnrampStatuses() []OnrampStatus {
	return []OnrampStatus{OnrampConfirmed, OnrampSubmitted, OnrampCancelled, OnrampFailed}
}

func ParseReportedOnrampStatus(raw string) (OnrampStatus, error) {
	if !slices.Contains(ReportedOnrampStatuses(), OnrampStatus(raw)) {
		return "", errs.New(errs.CodeInvalidInput, "funding.ParseReportedOnrampStatus", slog.String("raw", raw))
	}
	return OnrampStatus(raw), nil
}

func OnrampSources(to OnrampStatus) []OnrampStatus {
	var from []OnrampStatus
	for _, s := range OnrampStatuses() {
		if s.To(to) == nil {
			from = append(from, s)
		}
	}
	return from
}

func (s OnrampStatus) Terminal() bool { return len(onrampTransitions()[s]) == 0 }

func RefuseOnrampToken(status OnrampStatus, wasOpened bool, expiresAt, now time.Time) error {
	const op = "funding.RefuseOnrampToken"
	lapsed := status == OnrampCreated && !expiresAt.After(now)
	if lapsed || (status == OnrampExpired && !wasOpened) {
		return errs.New(errs.CodeOnrampLinkExpired, op)
	}
	return errs.New(errs.CodeOnrampLinkInvalid, op, slog.String("status", string(status)))
}

const OnrampTokenBytes = 32

type OnrampToken struct{ raw [OnrampTokenBytes]byte }

func NewOnrampToken(random [OnrampTokenBytes]byte) OnrampToken { return OnrampToken{raw: random} }

func ParseOnrampToken(encoded string) (OnrampToken, error) {
	const op = "funding.ParseOnrampToken"
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(raw) != OnrampTokenBytes {
		return OnrampToken{}, errs.New(errs.CodeOnrampLinkInvalid, op)
	}
	return OnrampToken{raw: [OnrampTokenBytes]byte(raw)}, nil
}

func (t OnrampToken) Encode() string { return base64.RawURLEncoding.EncodeToString(t.raw[:]) }

func (t OnrampToken) Hash() []byte {
	sum := sha256.Sum256(t.raw[:])
	return sum[:]
}
