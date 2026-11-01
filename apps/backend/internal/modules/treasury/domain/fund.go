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
	FundRefunded  FundStatus = "refunded"
)

const MinFundMicros = 1_000_000

func fundTransitions() map[FundStatus][]FundStatus {
	return map[FundStatus][]FundStatus{
		FundCreated:   {FundSubmitted, FundFailed},
		FundSubmitted: {FundLanded, FundFailed},
		FundLanded:    {FundSettled, FundRefunded},
	}
}

func (s FundStatus) CanMoveTo(to FundStatus) bool { return slices.Contains(fundTransitions()[s], to) }

func NewFundRefundTxn(h UserTxnHeader, usdc Asset, amount money.Micros) (UserTxn, error) {
	out, err := amount.Delta(money.Micros{})
	if err != nil {
		return UserTxn{}, err
	}
	back := money.SignedMicrosFromInt64(-out.Int64())
	h.Kind, h.Status = UserFund, TxnFailed
	return NewUserTxn(h, []UserEntry{
		{Account: UserWallet, Asset: usdc, Amount: back},
		{Account: UserExternal, Asset: usdc, Amount: out},
		{Account: UserWallet, Asset: usdc, Amount: out},
		{Account: UserExternal, Asset: usdc, Amount: back},
	})
}

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
