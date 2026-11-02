package feed

import (
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Tokens struct {
	Amount   money.Units
	Decimals uint8
}

func NewTokens(amount money.Units, decimals uint8) Tokens {
	return Tokens{Amount: amount, Decimals: decimals}
}

func (t Tokens) Add(o Tokens) (Tokens, error) {
	if t.Decimals != o.Decimals {
		return Tokens{}, errs.New(errs.CodeInvalidInput, "feed.Tokens.Add",
			slog.Int("a_decimals", int(t.Decimals)), slog.Int("b_decimals", int(o.Decimals)))
	}
	sum, err := t.Amount.Add(o.Amount)
	if err != nil {
		return Tokens{}, err
	}
	return Tokens{Amount: sum, Decimals: t.Decimals}, nil
}

func (t Tokens) IsZero() bool { return t.Amount.IsZero() }

func (t Tokens) PriceFor(usdc money.Micros) money.Micros {
	return money.PricePerUnits(usdc, t.Amount, t.Decimals)
}
