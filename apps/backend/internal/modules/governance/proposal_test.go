package governance_test

import (
	"log/slog"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const xstockDecimals = 8

func buyDraft(t *testing.T) domain.Draft {
	t.Helper()
	g := testkit.NewIDs(7)
	cabal, err := ids.ParseCabalID(g.NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	proposer, err := ids.ParseUserID(g.NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	return domain.Draft{
		ID: ids.ProposalIDFrom(g.NewV7()), CabalID: cabal, ProposerID: proposer,
		Kind: domain.KindBuy, Symbol: "AAPLx", Mint: chain.SolanaAddress(aaplxMint),
		USDCMicros: money.MicrosFromUint64(25_000_000), Thesis: "Earnings next week.",
		QuoteOut: 105_000_000, ExpiresAt: time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC),
	}
}

func sellDraft(t *testing.T) domain.Draft {
	t.Helper()
	d := buyDraft(t)
	d.Kind, d.USDCMicros, d.TokenAmount = domain.KindSell, money.Micros{}, money.NewBaseUnits(3_000_000, xstockDecimals)
	return d
}

func TestNewProposal_keepsAValidBuyAndSell(t *testing.T) {
	t.Parallel()
	for _, d := range []domain.Draft{buyDraft(t), sellDraft(t)} {
		d.Thesis = strings.Repeat("é", domain.MaxThesisRunes)
		p, err := domain.NewProposal(d)
		if err != nil || p.Draft() != d {
			t.Errorf("NewProposal(%s) = %+v, %v, want the draft back", d.Kind, p.Draft(), err)
		}
	}
	d := buyDraft(t)
	d.Thesis = ""
	if _, err := domain.NewProposal(d); err != nil {
		t.Errorf("NewProposal with no thesis err = %v", err)
	}
}

func TestNewProposal_rejectsEachInvalidField(t *testing.T) {
	t.Parallel()
	tooBig := uint64(math.MaxInt64) + 1
	for name, tc := range map[string]struct {
		edit  func(*domain.Draft)
		field string
	}{
		"unknown kind":       {func(d *domain.Draft) { d.Kind = "agent_add" }, "kind"},
		"buy of zero":        {func(d *domain.Draft) { d.USDCMicros = money.Micros{} }, "amount"},
		"buy with tokens":    {func(d *domain.Draft) { d.TokenAmount = money.NewBaseUnits(1, xstockDecimals) }, "amount"},
		"buy past bigint":    {func(d *domain.Draft) { d.USDCMicros = money.MicrosFromUint64(tooBig) }, "amount"},
		"sell of zero":       {func(d *domain.Draft) { d.Kind, d.USDCMicros = domain.KindSell, money.Micros{} }, "amount"},
		"sell with usdc":     {func(d *domain.Draft) { d.Kind = domain.KindSell }, "amount"},
		"zero quote":         {func(d *domain.Draft) { d.QuoteOut = 0 }, "quote_out_amount"},
		"quote past bigint":  {func(d *domain.Draft) { d.QuoteOut = tooBig }, "quote_out_amount"},
		"thesis of 281":      {func(d *domain.Draft) { d.Thesis = strings.Repeat("a", domain.MaxThesisRunes+1) }, "thesis"},
		"thesis not utf-8":   {func(d *domain.Draft) { d.Thesis = "\xff" }, "thesis"},
		"sell past bigint":   {func(d *domain.Draft) { sellPast(d, tooBig) }, "amount"},
		"sell kept its buys": {func(d *domain.Draft) { sellPast(d, 1); d.USDCMicros = money.MicrosFromUint64(1) }, "amount"},
	} {
		d := buyDraft(t)
		tc.edit(&d)
		p, err := domain.NewProposal(d)
		if errs.CodeOf(err) != errs.CodeInvalidInput ||
			!slices.ContainsFunc(errs.Detail(err), slog.String("field", tc.field).Equal) ||
			p != (domain.Proposal{}) {
			t.Errorf("%s: NewProposal = %+v, %v, want invalid_input on %s", name, p.Draft(), err, tc.field)
		}
	}
}

func sellPast(d *domain.Draft, tokens uint64) {
	d.Kind, d.USDCMicros, d.TokenAmount = domain.KindSell, money.Micros{}, money.NewBaseUnits(tokens, xstockDecimals)
}
