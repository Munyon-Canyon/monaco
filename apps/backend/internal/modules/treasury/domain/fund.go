package domain

import (
	"log/slog"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type FundStatus string

const (
	FundCreated   FundStatus = "created"
	FundSubmitted FundStatus = "submitted"
	FundLanded    FundStatus = "landed"
	FundSettled   FundStatus = "settled"
	FundFailed    FundStatus = "failed"
)

const MinFundMicros = 1_000_000

func fundTransitions() map[FundStatus][]FundStatus {
	return map[FundStatus][]FundStatus{
		FundCreated:   {FundSubmitted, FundFailed},
		FundSubmitted: {FundLanded, FundFailed},
		FundLanded:    {FundSettled},
	}
}

func (s FundStatus) CanMoveTo(to FundStatus) bool { return slices.Contains(fundTransitions()[s], to) }

func ParseFundAmount(raw string) (money.Micros, error) {
	const op = "treasury.ParseFundAmount"
	amount, err := money.ParseMicros(raw)
	if err != nil {
		return money.Micros{}, errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	if amount.Cmp(money.MicrosFromUint64(MinFundMicros)) < 0 {
		return money.Micros{}, errs.New(errs.CodeInvalidInput, op,
			slog.String("amount_micros", amount.String()), slog.Int("min_micros", MinFundMicros))
	}
	return amount, nil
}
