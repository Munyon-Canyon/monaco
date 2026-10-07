package domain

import (
	"log/slog"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type Status string

const (
	StatusOpen             Status = "open"
	StatusPassed           Status = "passed"
	StatusFailed           Status = "failed"
	StatusExpired          Status = "expired"
	StatusWithdrawn        Status = "withdrawn"
	StatusVoided           Status = "voided"
	StatusExecuted         Status = "executed"
	StatusExecutionBlocked Status = "execution_blocked"
)

func Statuses() []Status {
	return []Status{
		StatusOpen, StatusPassed, StatusFailed, StatusExpired,
		StatusWithdrawn, StatusVoided, StatusExecuted, StatusExecutionBlocked,
	}
}

func ParseStatus(raw string) (Status, error) {
	if !slices.Contains(Statuses(), Status(raw)) {
		return "", unknown("governance.ParseStatus", raw)
	}
	return Status(raw), nil
}

type Event string

const (
	EventPass     Event = "pass"
	EventFail     Event = "fail"
	EventExpire   Event = "expire"
	EventWithdraw Event = "withdraw"
	EventVoid     Event = "void"
	EventExecute  Event = "execute"
	EventBlock    Event = "block"
	EventReopen   Event = "reopen"
)

func Events() []Event {
	return []Event{EventPass, EventFail, EventExpire, EventWithdraw, EventVoid, EventExecute, EventBlock, EventReopen}
}

func transitions() map[Status]map[Event]Status {
	return map[Status]map[Event]Status{
		StatusOpen: {
			EventPass:     StatusPassed,
			EventFail:     StatusFailed,
			EventExpire:   StatusExpired,
			EventWithdraw: StatusWithdrawn,
			EventVoid:     StatusVoided,
		},
		StatusPassed: {
			EventExecute: StatusExecuted,
			EventBlock:   StatusExecutionBlocked,
			EventVoid:    StatusVoided,
		},
		StatusExecutionBlocked: {
			EventReopen: StatusPassed,
		},
	}
}

func Next(from Status, e Event) (Status, error) {
	to, ok := transitions()[from][e]
	if !ok {
		return from, errs.New(errs.CodeVersionConflict, "governance.Next",
			slog.String("from", string(from)), slog.String("event", string(e)))
	}
	return to, nil
}

func Sources(e Event) []Status {
	var from []Status
	for _, s := range Statuses() {
		if _, ok := transitions()[s][e]; ok {
			from = append(from, s)
		}
	}
	return from
}

func unknown(op, raw string) error {
	return errs.New(errs.CodeDecodeFailed, op, slog.String("raw", raw))
}
