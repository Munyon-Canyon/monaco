package app

import (
	"fmt"
	"log/slog"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	maxThesisRunes = 120
	hoursPerDay    = 24
	hourlyUntil    = 48 * time.Hour

	preIPOKind = "pre_ipo"

	outcomeFilled     = "filled"
	outcomeFillFailed = "fill_failed"
	outcomeFailed     = "failed"
	outcomeExpired    = "expired"
)

func side(action string) (string, error) {
	switch action {
	case "buy":
		return "Buy", nil
	case "sell":
		return "Sell", nil
	}
	return "", errs.New(errs.CodeInvalidInput, opTrade, slog.String("action", action))
}

func assetLabel(asset market.Asset) string {
	if string(asset.Kind) == preIPOKind {
		return asset.DisplayName
	}
	return strings.TrimSuffix(asset.Symbol, "x")
}

func dollars(m money.Micros) string {
	cents := m.Uint64() / microsPerCent
	if cents%100 == 0 {
		return fmt.Sprintf("$%d", cents/100)
	}
	return fmt.Sprintf("$%d.%02d", cents/100, cents%100)
}

func cents(c *big.Int) string {
	whole, frac := new(big.Int).QuoRem(c, big.NewInt(100), new(big.Int))
	return fmt.Sprintf("$%s.%02d", whole, frac.Int64())
}

func pow10(n uint8) *big.Int { return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil) }

func roundedQuo(num, den *big.Int) *big.Int {
	twice := new(big.Int).Lsh(num, 1)
	twice.Add(twice, den)
	return twice.Quo(twice, new(big.Int).Lsh(den, 1))
}

func units(amount uint64, decimals uint8) string {
	hundredths := roundedQuo(new(big.Int).Mul(new(big.Int).SetUint64(amount), big.NewInt(100)), pow10(decimals))
	whole, frac := new(big.Int).QuoRem(hundredths, big.NewInt(100), new(big.Int))
	out := whole.String()
	if frac.Sign() != 0 {
		out += strings.TrimRight(fmt.Sprintf(".%02d", frac.Int64()), "0")
	}
	return out
}

func fillPrice(usdc money.Micros, amount uint64, decimals uint8) string {
	if amount == 0 {
		return cents(new(big.Int))
	}
	num := new(big.Int).Mul(new(big.Int).SetUint64(usdc.Uint64()), pow10(decimals))
	num.Mul(num, big.NewInt(100))
	den := new(big.Int).Mul(new(big.Int).SetUint64(amount), big.NewInt(1_000_000))
	return cents(roundedQuo(num, den))
}

func closesIn(left time.Duration) string {
	left = left.Round(time.Minute)
	switch {
	case left < time.Hour:
		return strconv.Itoa(max(int(left/time.Minute), 0)) + "m"
	case left < hourlyUntil:
		return strconv.Itoa(int(left/time.Hour)) + "h"
	}
	return strconv.Itoa(int(left/time.Hour)/hoursPerDay) + "d"
}

func tally(yes, no int) string { return fmt.Sprintf("%d–%d", yes, no) }

func cutThesis(thesis string) string {
	runes := []rune(strings.TrimSpace(thesis))
	if len(runes) <= maxThesisRunes {
		return string(runes)
	}
	return string(runes[:maxThesisRunes]) + "…"
}

func createdTitle(proposer, side, asset, amount string) string {
	return fmt.Sprintf("%s proposed %s %s %s", proposer, side, asset, amount)
}

func createdBody(thesis, dur string) string {
	closes := "The vote closes in " + dur + "."
	if thesis = cutThesis(thesis); thesis != "" {
		return "“" + thesis + "” " + closes
	}
	return closes
}

func resultTitle(asset, verb, score string) string {
	return fmt.Sprintf("%s %s %s", asset, verb, score)
}

func filledBody(action, units, price, voted string) string {
	verb := "Bought"
	if action == "sell" {
		verb = "Sold"
	}
	return fmt.Sprintf("%s %s sh at %s. You voted %s.", verb, units, price, voted)
}

func fillFailedBody(action string, blocked bool, voted string) string {
	kept := "No money moved."
	if blocked && action == "sell" {
		kept = "The shares are still in the pot."
	}
	return fmt.Sprintf("The %s didn't go through. %s You voted %s.", action, kept, voted)
}

func failedBody(voted string) string { return "No trade. You voted " + voted + "." }

func expiredBody(voted string) string {
	return "Not enough votes by the close. No trade. You voted " + voted + "."
}
