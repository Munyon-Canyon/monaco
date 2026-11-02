package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type SellFill struct {
	SwapID    ids.SwapID
	CabalID   ids.CabalID
	Symbol    string
	Signature chain.Signature
	Recorded  money.Units
	OnChain   money.Units
	Code      errs.Code
}

func (f SellFill) Overstated() money.Units {
	over, err := f.Recorded.Sub(f.OnChain)
	if err != nil {
		return money.Units{}
	}
	return over
}

type CabalOverstatement struct {
	CabalID ids.CabalID
	Sells   int
	Micros  money.Units
}

type OverstatedSellsReport struct {
	Sells  []SellFill
	Unread []SellFill
	Cabals []CabalOverstatement
}

func OverstatedSells(
	ctx context.Context, reads sqlc.DBTX, reader ChainReader, usdc chain.Mint,
) (OverstatedSellsReport, error) {
	rows, err := sqlc.New(reads).ListConfirmedSellsInto(ctx, string(usdc.Address))
	if err != nil {
		return OverstatedSellsReport{}, err
	}
	var report OverstatedSellsReport
	for _, row := range rows {
		recorded, _, err := domain.ParseOptionalTokenAmount(row.OutAmount)
		if err != nil {
			return OverstatedSellsReport{}, err
		}
		fill := SellFill{
			SwapID: ids.SwapIDFrom(row.ID), CabalID: ids.CabalIDFrom(row.CabalID), Symbol: row.Symbol,
			Signature: chain.Signature(row.TxSignature.String), Recorded: recorded,
		}
		got, err := reader.InboundAmount(ctx, fill.Signature, chain.SolanaAddress(row.TreasuryAddress), usdc)
		if err != nil {
			fill.Code = errs.CodeOf(err)
			report.Unread = append(report.Unread, fill)
			continue
		}
		fill.OnChain = money.UnitsFromUint64(got.Uint64())
		if fill.Recorded.Cmp(fill.OnChain) > 0 {
			if err := report.add(fill); err != nil {
				return OverstatedSellsReport{}, err
			}
		}
	}
	return report, nil
}

func (r *OverstatedSellsReport) add(fill SellFill) error {
	r.Sells = append(r.Sells, fill)
	if n := len(r.Cabals); n == 0 || r.Cabals[n-1].CabalID != fill.CabalID {
		r.Cabals = append(r.Cabals, CabalOverstatement{CabalID: fill.CabalID})
	}
	last := &r.Cabals[len(r.Cabals)-1]
	last.Sells++
	total, err := last.Micros.Add(fill.Overstated())
	last.Micros = total
	return err
}
