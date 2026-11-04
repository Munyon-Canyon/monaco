package social_test

import (
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

func TestChatMoments_Joined_RejectsNilUser(t *testing.T) {
	t.Parallel()
	err := adapters.ChatMoments{}.Joined(t.Context(), db.Tx{}, events.CabalMemberJoined{V: 1}, time.Time{})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("a join with no user returned %v, want invalid_input", err)
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
			err := adapters.ChatMoments{}.ApplyFilled(t.Context(), db.Tx{}, e, asset, e.ConfirmedAt)
			if errs.CodeOf(err) != errs.CodeInvalidInput {
				t.Fatalf("returned %v, want invalid_input", err)
			}
		})
	}
}
