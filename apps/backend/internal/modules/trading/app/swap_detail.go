package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type SwapDetail struct {
	Swap  sqlc.SwapDetailRow
	Asset market.Asset
}

type SwapDetailReads struct {
	reads   sqlc.DBTX
	cabals  Cabals
	catalog Catalog
}

func NewSwapDetailReads(reads sqlc.DBTX, cabals Cabals, catalog Catalog) SwapDetailReads {
	return SwapDetailReads{reads: reads, cabals: cabals, catalog: catalog}
}

func (r SwapDetailReads) Swap(ctx context.Context, id ids.SwapID, user ids.UserID) (SwapDetail, error) {
	const op = "trading.SwapDetail"
	row, err := sqlc.New(r.reads).SwapDetail(ctx, id.UUID())
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return SwapDetail{}, errs.New(errs.CodeSwapNotFound, op)
	case err != nil:
		return SwapDetail{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	member, err := r.cabals.IsMember(ctx, ids.CabalIDFrom(row.CabalID), user)
	switch {
	case err != nil:
		return SwapDetail{}, err
	case !member && !publishedToFeed(row):
		return SwapDetail{}, errs.New(errs.CodeNotCabalMember, op)
	}
	row.Retryable = row.Retryable && member
	mint, err := market.ParseMint(row.TokenMint)
	if err != nil {
		return SwapDetail{}, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	asset, err := r.catalog.AssetByMint(ctx, mint)
	if err != nil {
		return SwapDetail{}, err
	}
	return SwapDetail{Swap: row, Asset: asset}, nil
}

func publishedToFeed(row sqlc.SwapDetailRow) bool {
	return row.SourceKind == string(domain.SourceProposal) && row.Status == string(domain.StatusConfirmed)
}
