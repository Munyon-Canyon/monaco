package domain

import (
	"log/slog"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const MaxPayoutAttempts = 3

type PayoutStatus string

const (
	PayoutSigned    PayoutStatus = "signed"
	PayoutBroadcast PayoutStatus = "broadcast"
	PayoutConfirmed PayoutStatus = "confirmed"
	PayoutExpired   PayoutStatus = "expired"
	PayoutFailed    PayoutStatus = "failed"
)

func ParsePayoutStatus(raw string) (PayoutStatus, error) {
	status := PayoutStatus(raw)
	if raw != "" && !slices.Contains([]PayoutStatus{
		PayoutSigned, PayoutBroadcast, PayoutConfirmed, PayoutExpired, PayoutFailed,
	}, status) {
		return "", errs.New(errs.CodeDecodeFailed, "treasury.ParsePayoutStatus", slog.String("raw", raw))
	}
	return status, nil
}

type PayoutAttempt struct {
	Number int16
	Status PayoutStatus
}

func (a PayoutAttempt) Live() bool {
	return a.Status == PayoutSigned || a.Status == PayoutBroadcast || a.Status == PayoutConfirmed
}

type PayoutStep uint8

const (
	PayoutIdle PayoutStep = iota + 1
	PayoutSign
	PayoutGiveUp
	PayoutSend
	PayoutCheck
	PayoutAwaitSale
)

func NextPayoutStep(job CashOutStatus, selling bool, latest PayoutAttempt) PayoutStep {
	switch {
	case job == CashOutStarted && !selling, job == CashOutPaying && !latest.Live():
		if latest.Number >= MaxPayoutAttempts {
			return PayoutGiveUp
		}
		return PayoutSign
	case job == CashOutStarted, job == CashOutSelling:
		return PayoutAwaitSale
	case job != CashOutPaying:
		return PayoutIdle
	case latest.Status == PayoutSigned:
		return PayoutSend
	default:
		return PayoutCheck
	}
}

type PayoutState uint8

const (
	PayoutNotFound PayoutState = iota + 1
	PayoutProcessing
	PayoutFinalized
)

type PayoutReading struct {
	State   PayoutState
	Failed  bool
	Expired bool
}

type PayoutVerdict uint8

const (
	PayoutInFlight PayoutVerdict = iota + 1
	PayoutMissing
	PayoutLanded
	PayoutRejected
	PayoutLapsed
)

func (r PayoutReading) Verdict() PayoutVerdict {
	switch {
	case r.State == PayoutFinalized && r.Failed:
		return PayoutRejected
	case r.State == PayoutFinalized:
		return PayoutLanded
	case r.State == PayoutNotFound && r.Expired:
		return PayoutLapsed
	case r.State == PayoutNotFound:
		return PayoutMissing
	default:
		return PayoutInFlight
	}
}
