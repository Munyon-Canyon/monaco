package app

import (
	"math"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestListingGaps_flagsOnlyLinkedListingsPricedOnSolanaAndSortsThem(t *testing.T) {
	t.Parallel()
	mint, err := domain.ParseMint("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
	if err != nil {
		t.Fatal(err)
	}
	listing := func(key string, linkedMint string, id byte) HeldListing {
		l := HeldListing{Key: ListingKey(key)}
		l.ID = multichainID(id)
		if linkedMint != "" {
			l.Asset = &ListedAsset{Mint: linkedMint}
		}
		return l
	}
	listings := map[multichain.ListingID]HeldListing{}
	priced := map[multichain.ListingID]money.Micros{}
	for i, l := range []HeldListing{
		listing("base:0x2", mint.String(), 1),
		listing("arbitrum:0x1", mint.String(), 2),
		listing("base:0x3", "", 3),
		listing("base:0x4", "TSLAmint", 4),
		listing("base:0x5", mint.String(), 5),
	} {
		listings[l.ID] = l
		priced[l.ID] = money.MicrosFromUint64([]uint64{110, 90, 500, 500, 101}[i] * 1_000_000)
	}
	gaps := listingGaps(listings, priced, map[domain.Mint]money.Micros{mint: money.MicrosFromUint64(100_000_000)})
	if len(gaps) != 2 || gaps[0].Listing.Key != "arbitrum:0x1" || gaps[0].GapBps != 1000 ||
		gaps[1].Listing.Key != "base:0x2" || gaps[1].GapBps != 1000 {
		t.Fatalf("gaps = %+v, want arbitrum then base at 10%%, none for unlinked, unpriced or 1%% off", gaps)
	}
	if got := GapBps(money.MicrosFromUint64(math.MaxUint64), money.Micros{}); got != math.MaxInt64 {
		t.Fatalf("GapBps over nothing = %d, want the largest gap", got)
	}
}

func multichainID(n byte) multichain.ListingID {
	var u uuid.UUID
	u[15] = n
	return multichain.ListingIDFrom(u)
}
