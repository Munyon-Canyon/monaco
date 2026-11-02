package adapters

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
)

func decodeNumbers(raw []byte, out *map[string]any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	return d.Decode(out)
}

func wireSwapJSON(t *testing.T, action, in, out string) map[string]any {
	t.Helper()
	got, err := wireSwap(app.SwapDetail{Swap: sqlc.SwapDetailRow{Action: action, InAmount: in, OutAmount: out}})
	if err != nil {
		t.Fatalf("wireSwap = %v", err)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := decodeNumbers(raw, &fields); err != nil {
		t.Fatal(err)
	}
	return fields
}

func TestWireSwap_aPastInt64FillIsNullAsAnIntegerAndExactAsAString(t *testing.T) {
	t.Parallel()
	buy := wireSwapJSON(t, "buy", "5000000", "25000000000000000000")
	if buy["token_amount"] != nil || buy["token_amount_str"] != "25000000000000000000" ||
		buy["usdc_micros"] != json.Number("5000000") {
		t.Fatalf("buy = token %v / %v, usdc %v", buy["token_amount"], buy["token_amount_str"], buy["usdc_micros"])
	}
	sell := wireSwapJSON(t, "sell", "25000000000000000000", "4000000000")
	if sell["token_amount"] != nil || sell["token_amount_str"] != "25000000000000000000" ||
		sell["usdc_micros"] != json.Number("4000000000") {
		t.Fatalf("sell = token %v / %v, usdc %v", sell["token_amount"], sell["token_amount_str"], sell["usdc_micros"])
	}
}

func TestWireSwap_anAmountThatFitsEqualsItsStringField(t *testing.T) {
	t.Parallel()
	got := wireSwapJSON(t, "buy", "5000000", "9223372036854775807")
	if got["token_amount"] != json.Number("9223372036854775807") || got["token_amount_str"] != "9223372036854775807" {
		t.Fatalf("token_amount = %v, token_amount_str = %v", got["token_amount"], got["token_amount_str"])
	}
	pending := wireSwapJSON(t, "buy", "5000000", "")
	if pending["token_amount"] != nil {
		t.Fatalf("token_amount before the fill = %v, want null", pending["token_amount"])
	}
	if _, ok := pending["token_amount_str"]; ok {
		t.Fatalf("token_amount_str before the fill = %v, want it absent", pending["token_amount_str"])
	}
}
