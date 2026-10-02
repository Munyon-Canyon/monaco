package app

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type listedAsset struct{ asset domain.Asset }

func (l listedAsset) AssetByID(context.Context, domain.AssetID) (domain.Asset, error) {
	return l.asset, nil
}

type refuseQuote struct{ t *testing.T }

func (r refuseQuote) Quote(context.Context, domain.Mint, domain.Mint, money.BaseUnits) (Quote, error) {
	r.t.Fatal("CheckRoute quoted after the mint failed to parse")
	return Quote{}, nil
}

func TestCheckRoute_badUSDCMint(t *testing.T) {
	t.Parallel()
	id, err := domain.ParseAssetID("01920000-0000-7000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	mint, err := domain.ParseMint("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
	if err != nil {
		t.Fatal(err)
	}
	asset := domain.Asset{
		ID: id, Symbol: "AAPLx", Mint: mint, Decimals: 8,
		ChainChecked: true, IssuerTradable: true, Override: domain.OverrideAuto,
	}
	checker := NewRouteChecker(listedAsset{asset}, refuseQuote{t}, clock.Real{})
	checker.usdcAddress = "not-a-mint"
	_, err = checker.CheckRoute(t.Context(), id, SideBuy, money.NewBaseUnits(1, usdcDecimals))
	if errs.CodeOf(err) != errs.CodeInvalidAddress {
		t.Fatalf("err = %v, want invalid_address", err)
	}
}
