package adapters

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	pastInt64   = "25000000000000000000"
	atInt64Max  = "9223372036854775807"
	justPastMax = "9223372036854775808"
)

func decodeNumbers(raw []byte, out *map[string]any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	return d.Decode(out)
}

func mustUnits(t *testing.T, raw string) money.Units {
	t.Helper()
	u, err := money.ParseUnits(raw)
	if err != nil {
		t.Fatalf("ParseUnits(%q) = %v", raw, err)
	}
	return u
}

func jsonOf(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal = %v", err)
	}
	var out map[string]any
	if err := decodeNumbers(raw, &out); err != nil {
		t.Fatalf("Unmarshal = %v", err)
	}
	return out
}

func TestWireProposal_aPastInt64AmountIsExactInTheStringFieldsAndNullInTheIntegerFields(t *testing.T) {
	t.Parallel()
	view := app.ProposalView{
		Kind:        domain.KindSell,
		Symbol:      "AAPLx",
		Status:      domain.StatusOpen,
		TokenAmount: mustUnits(t, pastInt64),
		QuoteOut:    mustUnits(t, "4000000000000000000000"),
		ExpiresAt: time.Date(
			2026,
			11,
			2,
			12,
			0,
			0,
			0,
			time.UTC,
		),
		CreatedAt: time.Date(2026, 11, 2, 11, 0, 0, 0, time.UTC),
	}
	got := jsonOf(t, wireProposal(view))
	want := map[string]any{
		"token_amount": nil, "token_amount_str": pastInt64,
		"quote_out_amount": nil, "quote_out_amount_str": "4000000000000000000000",
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s = %v, want %v", key, got[key], value)
		}
	}
}

func TestWireProposal_anAmountThatFitsEqualsItsStringField(t *testing.T) {
	t.Parallel()
	view := app.ProposalView{
		Kind: domain.KindSell, Symbol: "AAPLx", Status: domain.StatusOpen,
		TokenAmount: mustUnits(t, atInt64Max), QuoteOut: mustUnits(t, "105000000"),
	}
	got := jsonOf(t, wireProposal(view))
	if got["token_amount"] != json.Number("9223372036854775807") || got["token_amount_str"] != atInt64Max {
		t.Errorf("token_amount = %v, token_amount_str = %v", got["token_amount"], got["token_amount_str"])
	}
	if got["quote_out_amount"] != json.Number("105000000") || got["quote_out_amount_str"] != "105000000" {
		t.Errorf(
			"quote_out_amount = %v, quote_out_amount_str = %v",
			got["quote_out_amount"],
			got["quote_out_amount_str"],
		)
	}
	past := jsonOf(t, wireProposal(app.ProposalView{TokenAmount: mustUnits(t, justPastMax)}))
	if past["token_amount"] != nil || past["token_amount_str"] != justPastMax {
		t.Errorf(
			"one past int64: token_amount = %v, token_amount_str = %v",
			past["token_amount"],
			past["token_amount_str"],
		)
	}
}

func TestWireProposal_aBuyHasNoTokenAmount(t *testing.T) {
	t.Parallel()
	got := jsonOf(t, wireProposal(app.ProposalView{Kind: domain.KindBuy, USDCMicros: 25_000_000}))
	if _, ok := got["token_amount_str"]; ok {
		t.Errorf("token_amount_str = %v, want it absent on a buy", got["token_amount_str"])
	}
	if got["token_amount"] != nil {
		t.Errorf("token_amount = %v, want null", got["token_amount"])
	}
}

func TestWireAdminProposal_aPastInt64QuoteIsZeroInTheIntegerFieldAndExactInTheString(t *testing.T) {
	t.Parallel()
	view := app.ProposalView{
		Kind:        domain.KindSell,
		QuoteOut:    mustUnits(t, pastInt64),
		TokenAmount: mustUnits(t, pastInt64),
	}
	got := jsonOf(t, wireAdminProposal(app.AdminProposal{Detail: app.ProposalDetail{Proposal: view}}))
	if got["quote_out_amount"] != json.Number("0") || got["quote_out_amount_str"] != pastInt64 ||
		got["token_amount"] != nil || got["token_amount_str"] != pastInt64 {
		t.Fatalf("admin proposal = %v", got)
	}
	empty := jsonOf(t, wireAdminProposal(app.AdminProposal{}))
	if empty["quote_out_amount_str"] != "0" {
		t.Fatalf("quote_out_amount_str on a rule proposal = %v, want \"0\"", empty["quote_out_amount_str"])
	}
}

func TestWireOpenedProposal_aPastInt64SellIsExactInTheStringFields(t *testing.T) {
	t.Parallel()
	opened := app.OpenedProposal{
		Draft:       domain.Draft{Kind: domain.KindSell, Symbol: "AAPLx"},
		TokenAmount: mustUnits(t, pastInt64), QuoteOutAmount: mustUnits(t, "4000000000000000000000"),
	}
	got := jsonOf(t, wireOpenedProposal(opened, "Friday", app.ProposerCard{}))
	if got["token_amount"] != nil || got["token_amount_str"] != pastInt64 ||
		got["quote_out_amount"] != json.Number("0") || got["quote_out_amount_str"] != "4000000000000000000000" {
		t.Fatalf("opened proposal = %v", got)
	}
	fits := jsonOf(t, wireOpenedProposal(app.OpenedProposal{
		TokenAmount: mustUnits(t, "150000000"), QuoteOutAmount: mustUnits(t, "105000000"),
	}, "Friday", app.ProposerCard{}))
	if fits["token_amount"] != json.Number("150000000") || fits["token_amount_str"] != "150000000" ||
		fits["quote_out_amount"] != json.Number("105000000") || fits["quote_out_amount_str"] != "105000000" {
		t.Fatalf("opened proposal that fits = %v", fits)
	}
}

func TestWireFill_aPastInt64FillIsExactInTheStringField(t *testing.T) {
	t.Parallel()
	var swap api.LinkedSwap
	wireFill(&swap, app.Fill{TokenAmount: mustUnits(t, pastInt64), TokenDecimals: 18})
	got := jsonOf(t, swap)
	if got["token_amount"] != nil || got["token_amount_str"] != pastInt64 {
		t.Fatalf("fill = %v", got)
	}
}

func TestTokenAmount_readsTheIntegerOrTheStringButNotBoth(t *testing.T) {
	t.Parallel()
	five, text, bad := int64(5), pastInt64, "1.5"
	got, err := tokenAmount(&five, nil)
	if err != nil || got.String() != "5" {
		t.Fatalf("tokenAmount(5, nil) = %s, %v", got, err)
	}
	got, err = tokenAmount(nil, &text)
	if err != nil || got.String() != pastInt64 {
		t.Fatalf("tokenAmount(nil, past) = %s, %v", got, err)
	}
	got, err = tokenAmount(nil, nil)
	if err != nil || !got.IsZero() {
		t.Fatalf("tokenAmount(nil, nil) = %s, %v", got, err)
	}
	if _, err = tokenAmount(&five, &text); err == nil {
		t.Fatal("tokenAmount with both set succeeded, want invalid_input")
	}
	if _, err = tokenAmount(nil, &bad); err == nil {
		t.Fatal("tokenAmount(\"1.5\") succeeded, want invalid_input")
	}
}

func TestWirePendingVote_aPastInt64SellIsExactInTheStringField(t *testing.T) {
	t.Parallel()
	got := jsonOf(t, wirePendingVote(app.PendingVote{Kind: domain.KindSell, TokenAmount: mustUnits(t, pastInt64)}))
	if got["token_amount"] != nil || got["token_amount_str"] != pastInt64 {
		t.Fatalf("pending vote = %v", got)
	}
	fits := jsonOf(t, wirePendingVote(app.PendingVote{Kind: domain.KindSell, TokenAmount: mustUnits(t, "150000000")}))
	if fits["token_amount"] != json.Number("150000000") || fits["token_amount_str"] != "150000000" {
		t.Fatalf("pending vote that fits = %v", fits)
	}
}

func TestWireProposal_goldenJSONForAPastInt64Sell(t *testing.T) {
	t.Parallel()
	view := app.ProposalView{
		Kind:          domain.KindSell,
		Symbol:        "AAPLx",
		Status:        domain.StatusOpen,
		TokenDecimals: 18,
		TokenAmount:   mustUnits(t, pastInt64),
		QuoteOut:      mustUnits(t, "4000000000"),
		ExpiresAt: time.Date(
			2026,
			11,
			2,
			12,
			0,
			0,
			0,
			time.UTC,
		),
		CreatedAt: time.Date(2026, 11, 2, 11, 0, 0, 0, time.UTC),
	}
	got, err := json.Marshal(wireProposal(view))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/proposal_past_int64.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var gotIndented, wantIndented bytes.Buffer
	if err := json.Indent(&gotIndented, got, "", "  "); err != nil {
		t.Fatal(err)
	}
	if err := json.Indent(&wantIndented, bytes.TrimSpace(want), "", "  "); err != nil {
		t.Fatal(err)
	}
	if gotIndented.String() != wantIndented.String() {
		t.Fatalf("wireProposal JSON =\n%s\nwant\n%s", gotIndented.String(), wantIndented.String())
	}
}

func TestPreviewTrade_readsTheStringAmountAndRefusesBoth(t *testing.T) {
	t.Parallel()
	five, text := int64(5), pastInt64
	trade, err := previewTrade(api.GetCabalProposalPreviewParams{Kind: "sell", Symbol: "AAPLx", TokenAmountStr: &text})
	if err != nil || trade.TokenAmount.String() != pastInt64 {
		t.Fatalf("previewTrade(token_amount_str) = %s, %v", trade.TokenAmount, err)
	}
	if _, err = previewTrade(api.GetCabalProposalPreviewParams{
		Kind: "sell", Symbol: "AAPLx", TokenAmount: &five, TokenAmountStr: &text,
	}); err == nil {
		t.Fatal("previewTrade with both amounts succeeded, want invalid_input")
	}
}
