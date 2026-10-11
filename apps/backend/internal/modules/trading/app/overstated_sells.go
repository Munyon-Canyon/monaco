package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type SellFill struct {
	SwapID    ids.SwapID
	CabalID   ids.CabalID
	Symbol    string
	Signature chain.Signature
	Recorded  uint64
	OnChain   uint64
	Code      errs.Code
}

func (f SellFill) Overstated() uint64 { return f.Recorded - f.OnChain }

type CabalOverstatement struct {
	CabalID ids.CabalID
	Sells   int
	Micros  uint64
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
		fill := SellFill{
			SwapID: ids.SwapIDFrom(row.ID), CabalID: ids.CabalIDFrom(row.CabalID), Symbol: row.Symbol,
			Signature: chain.Signature(row.TxSignature.String), Recorded: uint64(max(row.OutAmount.Int64, 0)),
		}
		got, err := reader.InboundAmount(ctx, fill.Signature, chain.SolanaAddress(row.TreasuryAddress), usdc)
		if err != nil {
			fill.Code = errs.CodeOf(err)
			report.Unread = append(report.Unread, fill)
			continue
		}
		fill.OnChain = got.Uint64()
		if fill.Recorded > fill.OnChain {
			report.add(fill)
		}
	}
	return report, nil
}

func (r *OverstatedSellsReport) add(fill SellFill) {
	r.Sells = append(r.Sells, fill)
	if n := len(r.Cabals); n == 0 || r.Cabals[n-1].CabalID != fill.CabalID {
		r.Cabals = append(r.Cabals, CabalOverstatement{CabalID: fill.CabalID})
	}
	last := &r.Cabals[len(r.Cabals)-1]
	last.Sells++
	last.Micros += fill.Overstated()
}
