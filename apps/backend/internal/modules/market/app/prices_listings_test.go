package app

import (
	"math/big"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	v7Asset  = "01920000-0000-7000-8000-000000000001"
	v4Asset  = "6ba7b810-9dad-41d1-80b4-00c04fd430c8"
	evmToken = "0xB200000000000000000000000000000000000A01"
)

func TestParseListingKey_acceptsOnlyAnEVMNetworkAndAToken(t *testing.T) {
	t.Parallel()
	for asset, want := range map[string]bool{
		"base:0xb2": true,
		"XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp":        false,
		"solana:XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp": false,
		"dogechain:0xb2": false,
		"base:":          false,
	} {
		if _, _, ok := ParseListingKey(asset); ok != want {
			t.Fatalf("ParseListingKey(%q) ok = %v, want %v", asset, ok, want)
		}
	}
	if got := ListingKeyOf(chain.NetworkBase, evmToken); got != "base:0xb200000000000000000000000000000000000a01" {
		t.Fatalf("ListingKeyOf(base) = %s, want the token lowercased", got)
	}
	if got := ListingKeyOf(chain.NetworkSolana, "XsbE"); got != "solana:XsbE" {
		t.Fatalf("ListingKeyOf(solana) = %s, want the mint as is", got)
	}
}

func TestListingValue_isTokensTimesSharesPerTokenTimesThePrice(t *testing.T) {
	t.Parallel()
	price := ListingPrice{HeldListing: HeldListing{Key: "base:0xb2"}}
	price.Decimals, price.SharesPerToken = 18, big.NewRat(5, 4)
	half, _ := new(big.Int).SetString("500000000000000000", 10)
	if _, err := price.Value(half); errs.CodeOf(err) != errs.CodePricesStale {
		t.Fatalf("Value without a price = %v, want prices_stale", err)
	}
	price.Price = &domain.Sample{Micros: money.MicrosFromUint64(200_000_000)}
	if got, err := price.Value(half); err != nil || got.Uint64() != 125_000_000 {
		t.Fatalf("Value of 0.5 token at 1.25 shares and 200 USDC = %v, %v, want 125 USDC", got, err)
	}
	huge := new(big.Int).Lsh(big.NewInt(1), 80)
	if _, err := ListingValue(
		huge,
		0,
		big.NewRat(1, 1),
		money.MicrosFromUint64(1),
	); errs.CodeOf(
		err,
	) != errs.CodeInvalidInput {
		t.Fatalf("ListingValue past uint64 = %v, want invalid_input", err)
	}
}

func listingRow(t *testing.T) listingFields {
	t.Helper()
	id, err := uuid.Parse(v7Asset)
	if err != nil {
		t.Fatal(err)
	}
	return listingFields{
		ID: id, AssetID: &id, Issuer: "ondo", Symbol: "AAPLon", Network: "base", TokenAddress: "0xb2",
		Decimals: 18, SharesPerToken: "1.25", Venue: "uniswap", Key: "base:0xb2",
		Asset: ListedAsset{Mint: "XsbE", Symbol: "AAPLx"},
	}
}

func TestListingOf_refusesARowItCannotRead(t *testing.T) {
	t.Parallel()
	v4, err := uuid.Parse(v4Asset)
	if err != nil {
		t.Fatal(err)
	}
	for name, spoil := range map[string]func(*listingFields){
		"network":          func(r *listingFields) { r.Network = "dogechain" },
		"decimals":         func(r *listingFields) { r.Decimals = -1 },
		"shares per token": func(r *listingFields) { r.SharesPerToken = "0" },
		"not a number":     func(r *listingFields) { r.SharesPerToken = "x" },
		"asset id":         func(r *listingFields) { r.AssetID = &v4 },
	} {
		row := listingRow(t)
		spoil(&row)
		if _, err := listingOf(row); errs.CodeOf(err) != errs.CodeDecodeFailed {
			t.Fatalf("%s: listingOf = %v, want decode_failed", name, err)
		}
	}
	l, err := listingOf(listingRow(t))
	if err != nil || l.AssetID == nil || l.AssetID.String() != v7Asset || l.Asset.Symbol != "AAPLx" ||
		l.SharesPerToken.String() != "5/4" || l.Network != chain.NetworkBase {
		t.Fatalf("listingOf = %+v, %v", l, err)
	}
}
