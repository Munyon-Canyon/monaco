package governance_test

import (
	"context"
	"errors"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type marketListings struct {
	id  multichain.ListingID
	err error
}

func (m marketListings) ListingOn(
	context.Context, multichain.StockID, chain.Network,
) (multichain.ListingID, multichain.Venue, error) {
	if m.err != nil {
		return multichain.ListingID{}, "", m.err
	}
	return m.id, multichain.VenueUniswap, nil
}

func TestPinListings_reportsAMissingListingAsNotFoundAndPassesOtherFailures(t *testing.T) {
	t.Parallel()
	listing := multichain.ListingIDFrom(testkit.NewIDs(5367).NewV7())
	lookup := errs.New(errs.CodeInternal, "test")
	for name, tc := range map[string]struct {
		market marketListings
		found  bool
		err    error
	}{
		"listed":     {marketListings{id: listing}, true, nil},
		"not listed": {marketListings{err: errs.New(errs.CodeAssetNotFound, "test")}, false, nil},
		"failed":     {marketListings{err: lookup}, false, lookup},
	} {
		id, venue, found, err := adapters.PinListings{Market: tc.market}.PinListing(
			t.Context(), multichain.StockID{}, chain.NetworkBase,
		)
		if found != tc.found || !errors.Is(err, tc.err) ||
			(found && (id != listing || venue != multichain.VenueUniswap)) {
			t.Errorf("%s: PinListing = %s, %s, %t, %v", name, id, venue, found, err)
		}
	}
}
