package domain

import (
	"log/slog"
	"math"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type Status string

const (
	StatusCreated   Status = "created"
	StatusSubmitted Status = "submitted"
	StatusConfirmed Status = "confirmed"
	StatusFailed    Status = "failed"
)

func Statuses() []Status {
	return []Status{StatusCreated, StatusSubmitted, StatusConfirmed, StatusFailed}
}

type EventKind string

const (
	KindSubmit  EventKind = "submit"
	KindConfirm EventKind = "confirm"
	KindFail    EventKind = "fail"
)

type FailureCode string

const (
	FailureNeverSubmitted   FailureCode = "never_submitted"
	FailureBlockhashExpired FailureCode = "blockhash_expired"
	FailureJupiterFailed    FailureCode = "jupiter_failed"
	FailureForceResolved    FailureCode = "force_resolved"
	FailureSourceCancelled  FailureCode = "source_cancelled"
)

func FailureCodes() []FailureCode {
	return []FailureCode{
		FailureNeverSubmitted,
		FailureBlockhashExpired,
		FailureJupiterFailed,
		FailureForceResolved,
		FailureSourceCancelled,
	}
}

type Event struct {
	kind    EventKind
	failure FailureCode
}

func Submit() Event { return Event{kind: KindSubmit} }

func Confirm() Event { return Event{kind: KindConfirm} }

func Fail(code FailureCode) Event { return Event{kind: KindFail, failure: code} }

func (e Event) Kind() EventKind { return e.kind }

func (e Event) Failure() FailureCode { return e.failure }

func transitions() map[Status]map[EventKind]Status {
	return map[Status]map[EventKind]Status{
		StatusCreated:   {KindSubmit: StatusSubmitted, KindFail: StatusFailed},
		StatusSubmitted: {KindConfirm: StatusConfirmed, KindFail: StatusFailed},
	}
}

func failsFrom() map[FailureCode]Status {
	return map[FailureCode]Status{
		FailureNeverSubmitted:   StatusCreated,
		FailureBlockhashExpired: StatusSubmitted,
		FailureJupiterFailed:    StatusSubmitted,
		FailureForceResolved:    StatusSubmitted,
		FailureSourceCancelled:  StatusSubmitted,
	}
}

func Next(from Status, ev Event) (Status, error) {
	to, ok := transitions()[from][ev.kind]
	if ok && ev.kind == KindFail {
		ok = failsFrom()[ev.failure] == from
	}
	if !ok {
		return from, errs.New(errs.CodeVersionConflict, "trading.Next", slog.String("from", string(from)),
			slog.String("event", string(ev.kind)), slog.String("failure", string(ev.failure)))
	}
	return to, nil
}

func ParseStatus(raw string) (Status, error) {
	if !slices.Contains(Statuses(), Status(raw)) {
		return "", unknown("trading.ParseStatus", raw)
	}
	return Status(raw), nil
}

func ParseFailureCode(raw string) (FailureCode, error) {
	if !slices.Contains(FailureCodes(), FailureCode(raw)) {
		return "", unknown("trading.ParseFailureCode", raw)
	}
	return FailureCode(raw), nil
}

func unknown(op, raw string) error {
	return errs.New(errs.CodeDecodeFailed, op, slog.String("raw", raw))
}

func ParseAmount(v int64) (uint64, error) {
	if v < 0 {
		return 0, errs.New(errs.CodeDecodeFailed, "trading.ParseAmount", slog.Int64("raw", v))
	}
	return uint64(v), nil
}

func Column(v uint64) (int64, bool) {
	if v > math.MaxInt64 {
		return 0, false
	}
	return int64(v), true
}
