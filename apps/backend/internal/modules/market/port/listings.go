package port

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

type Listings interface {
	Listing(ctx context.Context, id multichain.ListingID) (multichain.Listing, error)
	ListingByAsset(ctx context.Context, asset string) (multichain.Listing, error)
}
