package adapters

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestWireFeedItem_aPastInt64FillKeepsItsDecimalsAndIsExactAsAString(t *testing.T) {
	t.Parallel()
	wide, err := money.ParseUnits("25000000000000000000")
	if err != nil {
		t.Fatal(err)
	}
	decimals := uint8(18)
	got := wireFeedItem(app.FeedItem{
		TokenDecimals: &decimals,
		TokenAmount:   wide,
		FilledTokens:  feed.NewTokens(wide, 18),
	})
	if got.TokenAmount != nil || got.FilledTokenAmount != nil {
		t.Fatalf("integer fields = %v and %v, want null past int64", got.TokenAmount, got.FilledTokenAmount)
	}
	if got.TokenAmountStr == nil || *got.TokenAmountStr != "25000000000000000000" ||
		got.FilledTokenAmountStr == nil || *got.FilledTokenAmountStr != "25000000000000000000" {
		t.Fatalf("string fields = %v and %v", got.TokenAmountStr, got.FilledTokenAmountStr)
	}
	if got.TokenDecimals == nil || *got.TokenDecimals != 18 {
		t.Fatalf("token_decimals = %v, want 18", got.TokenDecimals)
	}
}

func TestWireFeedItem_aFillThatFitsEqualsItsStringField(t *testing.T) {
	t.Parallel()
	decimals := uint8(8)
	got := wireFeedItem(app.FeedItem{
		TokenDecimals: &decimals, FilledTokens: feed.NewTokens(money.UnitsFromUint64(42_000_000), 8),
	})
	if got.FilledTokenAmount == nil || *got.FilledTokenAmount != 42_000_000 ||
		got.FilledTokenAmountStr == nil || *got.FilledTokenAmountStr != "42000000" {
		t.Fatalf("filled = %v and %v", got.FilledTokenAmount, got.FilledTokenAmountStr)
	}
	if got.TokenAmount != nil || got.TokenAmountStr != nil {
		t.Fatalf("token amount = %v and %v, want none", got.TokenAmount, got.TokenAmountStr)
	}
}
