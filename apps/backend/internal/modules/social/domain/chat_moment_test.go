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

func (m momentFixture) settled() domain.ProposalSettled {
	return domain.ProposalSettled{
		Outcome: domain.SettledFailed, ProposalID: m.prop, Side: domain.TradeBuy, Symbol: "NVDA",
		USDCMicros: 300_000_000, Yes: 2, No: 4, Voters: 6, ProposerID: m.user,
	}
}

func TestChatMoment_proposalSettledRoundTrips(t *testing.T) {
	t.Parallel()
	fx := newMomentFixture()
	sell := fx.settled()
	sell.Outcome, sell.Side, sell.USDCMicros, sell.TokenAmount = domain.SettledExpired, domain.TradeSell, 0, 2_000_000
	for _, in := range []domain.ProposalSettled{fx.settled(), sell} {
		moment, err := domain.NewProposalSettledMoment(in)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := moment.JSON()
		if err != nil {
			t.Fatal(err)
		}
		back, err := domain.ParseChatMoment(raw)
		if err != nil || back != moment || back.Settled() != in || back.Kind() != domain.MomentProposalSettled {
			t.Fatalf("round trip of %s = %+v, %v, want %+v", raw, back.Settled(), err, in)
		}
	}
}

func TestChatMoment_proposalSettledRefusesBadInput(t *testing.T) {
	t.Parallel()
	fx := newMomentFixture()
	mutate := func(f func(*domain.ProposalSettled)) domain.ProposalSettled {
		p := fx.settled()
		f(&p)
		return p
	}
	bad := map[string]domain.ProposalSettled{
		"outcome":  mutate(func(p *domain.ProposalSettled) { p.Outcome = "passed" }),
		"proposal": mutate(func(p *domain.ProposalSettled) { p.ProposalID = uuid.Nil }),
		"side":     mutate(func(p *domain.ProposalSettled) { p.Side = "hold" }),
		"symbol":   mutate(func(p *domain.ProposalSettled) { p.Symbol = " " }),
		"usdc":     mutate(func(p *domain.ProposalSettled) { p.USDCMicros = -1 }),
		"tokens":   mutate(func(p *domain.ProposalSettled) { p.TokenAmount = -1 }),
		"yes":      mutate(func(p *domain.ProposalSettled) { p.Yes = -1 }),
		"no":       mutate(func(p *domain.ProposalSettled) { p.No = -1 }),
		"voters":   mutate(func(p *domain.ProposalSettled) { p.Voters = -1 }),
		"proposer": mutate(func(p *domain.ProposalSettled) { p.ProposerID = ids.UserID{} }),
	}
	for name, in := range bad {
		if _, err := domain.NewProposalSettledMoment(in); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("%s: err = %v, want invalid_input", name, err)
		}
	}
}

func TestParseChatMoment_refusesAnIncompleteSettledRow(t *testing.T) {
	t.Parallel()
	fx := newMomentFixture()
	whole := `"kind":"proposal_settled","outcome":"failed","side":"buy","symbol":"NVDA","usdc_micros":1`
	ref := `"proposal_id":"` + fx.prop.String() + `"`
	person := `"proposer":"` + fx.user.UUID().String() + `"`
	for name, raw := range map[string]string{
		"no proposal": `{` + whole + `,"yes":1,"no":1,"voters":2,` + person + `}`,
		"no yes":      `{` + whole + `,` + ref + `,"no":1,"voters":2,` + person + `}`,
		"no no":       `{` + whole + `,` + ref + `,"yes":1,"voters":2,` + person + `}`,
		"no voters":   `{` + whole + `,` + ref + `,"yes":1,"no":1,` + person + `}`,
		"no proposer": `{` + whole + `,` + ref + `,"yes":1,"no":1,"voters":2}`,
		"bad outcome": `{"kind":"proposal_settled","outcome":"passed","side":"buy","symbol":"NVDA",` + ref + `,"yes":1,"no":1,"voters":2,` + person + `}`,
	} {
		if _, err := domain.ParseChatMoment([]byte(raw)); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("%s: err = %v, want invalid_input", name, err)
		}
	}
}

func TestChatMoment_tradeFilledKeepsTheTallyAndTheProposer(t *testing.T) {
	t.Parallel()
	fx := newMomentFixture()
	in := fx.trade()
	in.Tallied, in.Yes, in.No, in.ProposerID = true, 5, 1, fx.user
	moment, err := domain.NewTradeFilledMoment(in)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := moment.JSON()
	if err != nil {
		t.Fatal(err)
	}
	back, err := domain.ParseChatMoment(raw)
	if err != nil || back.Trade() != moment.Trade() || !back.Trade().Tallied || back.Trade().ProposerID != fx.user {
		t.Fatalf("round trip of %s = %+v, %v, want %+v", raw, back.Trade(), err, moment.Trade())
	}
	in.Yes = -1
	if _, err := domain.NewTradeFilledMoment(in); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("a negative tally: err = %v, want invalid_input", err)
	}
	old := `{"kind":"trade_filled","side":"buy","symbol":"NVDA","asset_id":"` + fx.asset.String() + `","swap_id":"` +
		fx.swap.String() + `","filled_at":"2026-09-30T14:30:00Z","price_micros":1,"usdc_micros":1}`
	legacy, err := domain.ParseChatMoment([]byte(old))
	if err != nil || legacy.Trade().Tallied || legacy.Trade().ProposerID != (ids.UserID{}) {
		t.Fatalf("a row written before the tally = %+v, %v, want no tally and no proposer", legacy.Trade(), err)
	}
}
