package port

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

type Listings interface {
	Listing(ctx context.Context, id multichain.ListingID) (multichain.Listing, error)
}
