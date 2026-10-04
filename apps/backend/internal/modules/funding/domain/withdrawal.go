package domain

import (
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type WithdrawalStatus string

const (
	WithdrawalCreated   WithdrawalStatus = "created"
	WithdrawalSubmitted WithdrawalStatus = "submitted"
	WithdrawalConfirmed WithdrawalStatus = "confirmed"
	WithdrawalFailed    WithdrawalStatus = "failed"
)

const MinWithdrawalMicros = 1_000_000

type WithdrawalRequest struct {
	Amount money.Micros
	To     chain.SolanaAddress
}

func ParseWithdrawalRequest(rawAmount, rawTo string) (WithdrawalRequest, error) {
	const op = "funding.ParseWithdrawalRequest"
	amount, err := money.ParseMicros(rawAmount)
	if err != nil {
		return WithdrawalRequest{}, errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	if amount.Cmp(money.MicrosFromUint64(MinWithdrawalMicros)) < 0 {
		return WithdrawalRequest{}, errs.New(errs.CodeInvalidInput, op,
			slog.String("amount_micros", amount.String()), slog.Int("min_micros", MinWithdrawalMicros))
	}
	to, err := chain.ParseAddress(rawTo)
	if err != nil {
		return WithdrawalRequest{}, errs.Wrap(err, errs.CodeInvalidAddress, op)
	}
	return WithdrawalRequest{Amount: amount, To: to}, nil
}
