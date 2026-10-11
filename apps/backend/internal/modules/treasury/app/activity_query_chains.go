package app

import (
	"context"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
)

func (r *ActivityReads) WithListings(listings ListingReader) *ActivityReads {
	r.listings = listings
	return r
}

func (r *ActivityReads) listingNames(
	ctx context.Context, rows []sqlc.ListActivityRow, names map[domain.Asset]AssetName,
) error {
	if r.listings == nil {
		return nil
	}
	var keys []string
	for _, row := range rows {
		if _, known := names[domain.Asset(row.Asset.String)]; row.Asset.Valid && !known &&
			strings.Contains(row.Asset.String, ":") {
			keys = append(keys, row.Asset.String)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	quotes, err := r.listings(ctx, keys, time.Time{})
	if err != nil {
		return err
	}
	for key, q := range quotes {
		name := AssetName{Symbol: q.Symbol, Name: q.Symbol}
		if q.Asset != nil {
			name = AssetName{Symbol: q.Asset.Symbol, Name: q.Asset.DisplayName}
		}
		names[domain.Asset(key)] = name
	}
	return nil
}
