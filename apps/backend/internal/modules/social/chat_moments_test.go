package social_test

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestChatMoments_Joined_AcknowledgesNilUser(t *testing.T) {
	t.Parallel()
	err := adapters.ChatMoments{}.Joined(t.Context(), db.Tx{}, events.CabalMemberJoined{V: 1}, time.Time{})
	if err != nil {
		t.Fatalf("a join with no user returned %v, want nil", err)
	}
}

type failingAssets struct {
	app.Assets
	err error
}

func (f failingAssets) AssetByMint(context.Context, string) (app.AssetCard, error) {
	return app.AssetCard{}, f.err
}

func TestChatMoments_FetchFilled_AcknowledgesUnlistedMintsAndSurfacesOtherErrors(t *testing.T) {
	t.Parallel()
	buy := events.TradeConfirmed{V: 1, Action: "buy", OutMint: marketfake.AAPLx().Mint.Address()}
	notFound := adapters.ChatMoments{Assets: failingAssets{err: errs.New(errs.CodeAssetNotFound, "test")}}
	asset, err := notFound.FetchFilled(t.Context(), buy)
	if err != nil || asset.Asset.ID != uuid.Nil {
		t.Fatalf("unlisted mint fetched (%+v, %v), want a zero card and no error", asset, err)
	}
	down := adapters.ChatMoments{Assets: failingAssets{err: errs.New(errs.CodeInternal, "test")}}
	if _, err := down.FetchFilled(t.Context(), buy); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("a failing catalog returned %v, want internal", err)
	}
	if _, err := down.FetchFilled(t.Context(), events.TradeConfirmed{V: 1, Action: "swap"}); err != nil {
		t.Fatalf("a non-trade action returned %v, want nil", err)
	}
}

func TestChatMoments_ApplyFilled_AcknowledgesAnUnlistedMint(t *testing.T) {
	t.Parallel()
	e := events.TradeConfirmed{V: 1, Action: "buy"}
	err := adapters.ChatMoments{}.ApplyFilled(t.Context(), db.Tx{}, e, adapters.FilledFetch{}, e.ConfirmedAt)
	if err != nil {
		t.Fatalf("a fill with no asset returned %v, want nil", err)
	}
}

func TestChatMoments_ApplyFilled_RejectsUnwritableFills(t *testing.T) {
	t.Parallel()
	asset := app.AssetCard{ID: marketfake.AAPLx().ID.UUID(), Symbol: "AAPLx", Decimals: 1}
	base := events.TradeConfirmed{
		V: 1, SwapID: uuid.NewSHA1(uuid.Nil, []byte("swap")), CabalID: uuid.NewSHA1(uuid.Nil, []byte("cabal")),
		Action: "buy", OutAmount: money.UnitsFromUint64(20),
		USDCMicros: money.MicrosFromUint64(393_500_000), ConfirmedAt: time.Date(2026, 9, 30, 14, 0, 0, 0, time.UTC),
	}
	huge := base
	huge.USDCMicros = money.MicrosFromUint64(1 << 63)
	zero := base
	zero.OutAmount = money.UnitsFromUint64(0)
	vast := base
	vast.OutAmount, _ = money.UnitsFromBig(new(big.Int).Lsh(big.NewInt(1), 70))
	cases := map[string]events.TradeConfirmed{"usdc over int64": huge, "no shares": zero, "shares over uint64": vast}
	for name, e := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := adapters.ChatMoments{}.ApplyFilled(
				t.Context(),
				db.Tx{},
				e,
				adapters.FilledFetch{Asset: asset},
				e.ConfirmedAt,
			)
			if errs.CodeOf(err) != errs.CodeInvalidInput {
				t.Fatalf("returned %v, want invalid_input", err)
			}
		})
	}
}
