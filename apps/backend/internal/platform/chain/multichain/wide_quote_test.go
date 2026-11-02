package multichain_test

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

func TestQuote_grossOutputPastUint64KeepsItsExactValueThroughJSON(t *testing.T) {
	t.Parallel()
	wide, ok := new(
		big.Int,
	).SetString("115792089237316195423570985008687907853269984665640564039457584007913129639935", 10)
	if !ok {
		t.Fatal("bad fixture")
	}
	q := pinQuote(chain.NetworkBase)
	q.GrossOutput = wide
	raw, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	got := pinQuote(chain.NetworkBase)
	got.GrossOutput = nil
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.GrossOutput.Cmp(wide) != 0 {
		t.Fatalf("GrossOutput = %s, want %s", got.GrossOutput, wide)
	}
}
