package adapters

import (
	"context"
	"math"
	"math/big"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

type StockListings struct{ q *sqlc.Queries }

func NewStockListings(db sqlc.DBTX) StockListings { return StockListings{q: sqlc.New(db)} }

var _ app.RouteListings = StockListings{}

func (s StockListings) EnabledListings(ctx context.Context, stock multichain.StockID) ([]multichain.Listing, error) {
	const op = "market.StockListings.EnabledListings"
	rows, err := s.q.ListEnabledListingsForAsset(ctx, pgtype.UUID{Bytes: stock.UUID(), Valid: true})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	out := make([]multichain.Listing, 0, len(rows))
	for _, row := range rows {
		shares, _ := new(big.Rat).SetString(row.SharesPerToken)
		out = append(out, multichain.Listing{
			ID:      multichain.ListingIDFrom(row.ID),
			StockID: stock,
			Issuer:  row.Issuer,
			Symbol:  row.Symbol,
			Network: chain.Network(
				row.Network,
			),
			TokenAddress:   row.TokenAddress,
			WrapperAddress: row.WrapperAddress.String,
			Decimals: uint8(
				min(max(row.Decimals.Int16, 0), math.MaxUint8),
			),
			SharesPerToken: shares,
			Venue:          multichain.Venue(row.Venue),
		})
	}
	return out, nil
}
