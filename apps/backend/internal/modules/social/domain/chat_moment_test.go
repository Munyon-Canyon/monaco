package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type momentFixture struct {
	user  ids.UserID
	asset uuid.UUID
	swap  uuid.UUID
	prop  uuid.UUID
	at    time.Time
}

func newMomentFixture() momentFixture {
	g := testkit.NewIDs(7)
	return momentFixture{
		user: ids.NewUserID(g), asset: g.NewV7(), swap: g.NewV7(), prop: g.NewV7(),
		at: time.Date(2026, 9, 30, 14, 30, 0, 0, time.UTC),
	}
}

func (m momentFixture) trade() domain.TradeFilled {
	return domain.TradeFilled{
		Side: domain.TradeBuy, Symbol: "NVDA", AssetID: m.asset, PriceMicros: 196_750_000,
		USDCMicros: 300_000_000, FilledAt: m.at, SwapID: m.swap, ProposalID: m.prop,
	}
}

func TestChatMoment_roundTripsEveryKind(t *testing.T) {
	t.Parallel()
	fx := newMomentFixture()
	joined, err := domain.NewMemberJoinedMoment(fx.user)
	if err != nil {
		t.Fatal(err)
	}
	trade, err := domain.NewTradeFilledMoment(fx.trade())
	if err != nil {
		t.Fatal(err)
	}
	bare := fx.trade()
	bare.ProposalID = uuid.Nil
	untied, err := domain.NewTradeFilledMoment(bare)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []domain.ChatMoment{domain.NewCabalCreatedMoment(), joined, trade, untied} {
		raw, err := m.JSON()
		if err != nil {
			t.Fatal(err)
		}
		back, err := domain.ParseChatMoment(raw)
		if err != nil {
			t.Fatalf("parse %s: %v", raw, err)
		}
		if back != m {
			t.Fatalf("round trip of %s = %+v, want %+v", raw, back, m)
		}
	}
	if joined.Member() != fx.user || trade.Trade().Symbol != "NVDA" || trade.Kind() != domain.MomentTradeFilled {
		t.Fatalf("accessors read %+v %+v", joined, trade)
	}
}

func TestChatMoment_constructorsRefuseBadInput(t *testing.T) {
	t.Parallel()
	fx := newMomentFixture()
	mutate := func(f func(*domain.TradeFilled)) domain.TradeFilled {
		t := fx.trade()
		f(&t)
		return t
	}
	bad := map[string]domain.TradeFilled{
		"side":      mutate(func(t *domain.TradeFilled) { t.Side = "hold" }),
		"symbol":    mutate(func(t *domain.TradeFilled) { t.Symbol = " " }),
		"asset":     mutate(func(t *domain.TradeFilled) { t.AssetID = uuid.Nil }),
		"swap":      mutate(func(t *domain.TradeFilled) { t.SwapID = uuid.Nil }),
		"price":     mutate(func(t *domain.TradeFilled) { t.PriceMicros = 0 }),
		"usdc":      mutate(func(t *domain.TradeFilled) { t.USDCMicros = -1 }),
		"filled_at": mutate(func(t *domain.TradeFilled) { t.FilledAt = time.Time{} }),
	}
	for name, in := range bad {
		if _, err := domain.NewTradeFilledMoment(in); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("%s: err = %v, want invalid_input", name, err)
		}
	}
	if _, err := domain.NewMemberJoinedMoment(ids.UserID{}); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("member without a user: err = %v, want invalid_input", err)
	}
}

func TestParseChatMoment_refusesWhatItCannotRead(t *testing.T) {
	t.Parallel()
	fx := newMomentFixture()
	for name, raw := range map[string]string{
		"unknown kind":     `{"kind":"cabal_renamed"}`,
		"not json":         `nope`,
		"joined no user":   `{"kind":"member_joined"}`,
		"trade incomplete": `{"kind":"trade_filled","side":"buy","symbol":"NVDA"}`,
		"trade bad side":   `{"kind":"trade_filled","side":"x","symbol":"NVDA","asset_id":"` + fx.asset.String() + `","swap_id":"` + fx.swap.String() + `","filled_at":"2026-09-30T14:30:00Z","price_micros":1,"usdc_micros":1}`,
	} {
		if _, err := domain.ParseChatMoment([]byte(raw)); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("%s: err = %v, want invalid_input", name, err)
		}
	}
}

func TestChatMoment_zeroValueHasNoJSON(t *testing.T) {
	t.Parallel()
	if _, err := (domain.ChatMoment{}).JSON(); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("err = %v, want invalid_input", err)
	}
}
