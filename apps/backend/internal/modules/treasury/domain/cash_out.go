package domain

import (
	"log/slog"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
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
	CashOutSell            CashOutEvent = "sell"
	CashOutStartPaying     CashOutEvent = "start_paying"
	CashOutComplete        CashOutEvent = "complete"
	CashOutCompletePartial CashOutEvent = "complete_partial"
	CashOutFail            CashOutEvent = "fail"
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
	return []CashOutEvent{CashOutSell, CashOutStartPaying, CashOutComplete, CashOutCompletePartial, CashOutFail}
}

func cashOutTransitions() map[CashOutStatus]map[CashOutEvent]CashOutStatus {
	return map[CashOutStatus]map[CashOutEvent]CashOutStatus{
		CashOutStarted: {CashOutSell: CashOutSelling, CashOutStartPaying: CashOutPaying, CashOutFail: CashOutFailed},
		CashOutSelling: {CashOutStartPaying: CashOutPaying, CashOutFail: CashOutFailed},
		CashOutPaying: {
			CashOutComplete: CashOutCompleted, CashOutCompletePartial: CashOutPartial, CashOutFail: CashOutFailed,
		},
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

type SaleSettlement struct {
	Paid     money.Micros
	Unpaid   money.Micros
	Returned money.SharesUnits
	Status   CashOutStatus
}

func SettleSale(units money.SharesUnits, slice, paid money.Micros) (SaleSettlement, error) {
	const op = "treasury.SettleSale"
	unpaid, err := slice.Sub(paid)
	if err != nil {
		return SaleSettlement{}, errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	returned, err := money.MulDiv(units.Uint64(), unpaid.Uint64(), slice.Uint64())
	if err != nil {
		return SaleSettlement{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	s := SaleSettlement{
		Paid: paid, Unpaid: unpaid, Returned: money.SharesUnitsFromUint64(returned), Status: CashOutPaying,
	}
	if paid.IsZero() {
		s.Status = CashOutFailed
	}
	return s, nil
}

func CashOutEnd(paid, slice money.Micros) CashOutEvent {
	if paid.Cmp(slice) < 0 {
		return CashOutCompletePartial
	}
	return CashOutComplete
}

func CashOutReturn(h UserTxnHeader, usdc Asset, s SaleSettlement) (UserTxn, error) {
	const op = "treasury.CashOutReturn"
	unpaid, err := s.Unpaid.Delta(money.Micros{})
	if err != nil {
		return UserTxn{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	entries := []UserEntry{
		{Account: UserCabal, Asset: usdc, Amount: unpaid},
		{Account: UserWallet, Asset: usdc, Amount: money.SignedMicrosFromInt64(-unpaid.Int64())},
	}
	if !s.Returned.IsZero() {
		returned, err := money.MicrosFromUint64(s.Returned.Uint64()).Delta(money.Micros{})
		if err != nil {
			return UserTxn{}, errs.Wrap(err, errs.CodeOf(err), op)
		}
		entries = append(entries,
			UserEntry{Account: UserHolder, Asset: SharesAsset(h.CabalID), Amount: returned},
			UserEntry{
				Account: UserIssuer, Asset: SharesAsset(h.CabalID),
				Amount: money.SignedMicrosFromInt64(-returned.Int64()),
			},
		)
	}
	return NewUserTxn(h, entries)
}
