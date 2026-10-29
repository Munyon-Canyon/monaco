package domain

import (
	"math/big"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	AuthorKindUser    = "user"
	AuthorKindSystem  = "system"
	SystemAuthorName  = "Monaco"
	ReasonSystemReply = "system_reply"
)

type SystemOutcome string

const (
	OutcomeFilled      SystemOutcome = "filled"
	OutcomeClosed      SystemOutcome = "closed"
	OutcomeTradeFailed SystemOutcome = "trade_failed"
)

func SystemReplyText(outcome SystemOutcome, side feed.Action, symbol string, before, after *money.BaseUnits) string {
	switch outcome {
	case OutcomeFilled:
		return filledText(symbol, before, after)
	case OutcomeTradeFailed:
		return "Passed, but the trade didn't go through. " + nothingDone(side)
	case OutcomeClosed:
		return "Closed without passing. " + nothingDone(side)
	default:
		return ""

	}
}

func nothingDone(side feed.Action) string {
	if side == feed.ActionSell {
		return "Nothing was sold."
	}
	return "Nothing was bought."
}

func filledText(symbol string, before, after *money.BaseUnits) string {
	if after == nil {
		return "Passed and filled."
	}
	var text strings.Builder
	text.WriteString("Passed and filled. ")
	if before != nil {
		text.WriteString(FormatShares(*before) + " → ")
	}
	text.WriteString(FormatShares(*after) + " sh " + symbol + " in the treasury.")
	return text.String()
}

func FormatShares(units money.BaseUnits) string {
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(units.Decimals())), nil)
	hundredths := new(big.Int).Mul(new(big.Int).SetUint64(units.Uint64()), big.NewInt(100))
	hundredths.Add(hundredths, new(big.Int).Rsh(scale, 1))
	hundredths.Quo(hundredths, scale)
	whole, cents := new(big.Int).QuoRem(hundredths, big.NewInt(100), new(big.Int))
	out := whole.String()
	if cents.Sign() == 0 {
		return out
	}
	frac := strings.TrimRight(leftPad(cents.String()), "0")
	return out + "." + frac
}

func leftPad(cents string) string {
	if len(cents) == 1 {
		return "0" + cents
	}
	return cents
}
