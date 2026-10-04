package domain

import (
	"log/slog"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type CashOutStatus string

const (
	CashOutStarted   CashOutStatus = "started"
	CashOutSelling   CashOutStatus = "selling"
	CashOutPaying    CashOutStatus = "paying"
	CashOutCompleted CashOutStatus = "completed"
	CashOutPartial   CashOutStatus = "partial"
	CashOutFailed    CashOutStatus = "failed"
)

type CashOutEvent string

const (
	CashOutStartPaying CashOutEvent = "start_paying"
	CashOutComplete    CashOutEvent = "complete"
	CashOutFail        CashOutEvent = "fail"
)

func CashOutStatuses() []CashOutStatus {
	return []CashOutStatus{
		CashOutStarted,
		CashOutSelling,
		CashOutPaying,
		CashOutCompleted,
		CashOutPartial,
		CashOutFailed,
	}
}

func CashOutEvents() []CashOutEvent {
	return []CashOutEvent{CashOutStartPaying, CashOutComplete, CashOutFail}
}

func cashOutTransitions() map[CashOutStatus]map[CashOutEvent]CashOutStatus {
	return map[CashOutStatus]map[CashOutEvent]CashOutStatus{
		CashOutStarted: {CashOutStartPaying: CashOutPaying, CashOutFail: CashOutFailed},
		CashOutPaying:  {CashOutComplete: CashOutCompleted, CashOutFail: CashOutFailed},
	}
}

func NextCashOut(from CashOutStatus, event CashOutEvent) (CashOutStatus, error) {
	if !slices.Contains(CashOutStatuses(), from) || !slices.Contains(CashOutEvents(), event) {
		return from, errs.New(errs.CodeDecodeFailed, "treasury.NextCashOut",
			slog.String("from", string(from)), slog.String("event", string(event)))
	}
	if to, ok := cashOutTransitions()[from][event]; ok {
		return to, nil
	}
	return from, errs.New(errs.CodeVersionConflict, "treasury.NextCashOut",
		slog.String("from", string(from)), slog.String("event", string(event)))
}

func ParseCashOutStatus(raw string) (CashOutStatus, error) {
	status := CashOutStatus(raw)
	if !slices.Contains(CashOutStatuses(), status) {
		return "", errs.New(errs.CodeDecodeFailed, "treasury.ParseCashOutStatus", slog.String("raw", raw))
	}
	return status, nil
}
