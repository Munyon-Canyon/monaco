package domain

import (
	"log/slog"
	"math/big"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type DepositSource string

const (
	SourceWebhook   DepositSource = "webhook"
	SourceReconcile DepositSource = "reconcile"
)

type ExternalDepositStatus string

const (
	ExternalDetected       ExternalDepositStatus = "detected"
	ExternalIgnoredDust    ExternalDepositStatus = "ignored_dust"
	ExternalIgnoredUnknown ExternalDepositStatus = "ignored_unknown"
)

const DustBelowMicros = 1_000_000

type Verdict int

const (
	VerdictOwn Verdict = iota
	VerdictUnknownAsset
	VerdictDust
	VerdictDetected
)

type Inbound struct {
	Owned bool
	Known bool
	Value *money.Micros
}

func Classify(in Inbound) Verdict {
	switch {
	case in.Owned:
		return VerdictOwn
	case !in.Known:
		return VerdictUnknownAsset
	case in.Value != nil && in.Value.Uint64() < DustBelowMicros:
		return VerdictDust
	default:
		return VerdictDetected
	}
}

func (v Verdict) Status() ExternalDepositStatus {
	return [...]ExternalDepositStatus{
		VerdictOwn: "", VerdictUnknownAsset: ExternalIgnoredUnknown, VerdictDust: ExternalIgnoredDust,
		VerdictDetected: ExternalDetected,
	}[v]
}

func (v Verdict) Code() errs.Code {
	return [...]errs.Code{
		VerdictOwn: "", VerdictUnknownAsset: errs.CodeUnknownAsset, VerdictDust: errs.CodeDust, VerdictDetected: "",
	}[v]
}

func StockValue(units money.BaseUnits, price money.Micros, num, den uint64) (money.Micros, error) {
	const op = "funding.StockValue"
	if den == 0 || num == 0 {
		return money.Micros{}, errs.New(errs.CodeInvalidInput, op)
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(units.Decimals())), nil)
	v := new(big.Int).SetUint64(units.Uint64())
	v.Mul(v, new(big.Int).SetUint64(price.Uint64()))
	v.Mul(v, new(big.Int).SetUint64(num))
	v.Quo(v, scale.Mul(scale, new(big.Int).SetUint64(den)))
	if !v.IsUint64() {
		return money.Micros{}, errs.New(errs.CodeInvalidInput, op, slog.String("value", v.String()))
	}
	return money.MicrosFromUint64(v.Uint64()), nil
}
