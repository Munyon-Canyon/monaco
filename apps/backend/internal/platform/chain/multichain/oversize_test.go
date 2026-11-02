package multichain_test

import (
	"encoding/json"
	"math"
	"math/big"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

func TestQuote_oversizeOnlyFlagsAnEVMOutputPastInt64(t *testing.T) {
	t.Parallel()
	past := new(big.Int).Add(big.NewInt(math.MaxInt64), big.NewInt(1))
	tests := []struct {
		name    string
		network chain.Network
		out     *big.Int
		want    bool
	}{
		{"base past int64", chain.NetworkBase, past, true},
		{"base at int64", chain.NetworkBase, big.NewInt(math.MaxInt64), false},
		{"base without output", chain.NetworkBase, nil, false},
		{"solana past int64", chain.NetworkSolana, past, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := multichain.Quote{Network: tc.network, GrossOutput: tc.out}
			if got := q.Oversize(); got != tc.want {
				t.Fatalf("Oversize = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestComparison_quoteForFindsAListingOnlyWhenRanked(t *testing.T) {
	t.Parallel()
	ranked := pinQuote(chain.NetworkBase)
	c := multichain.Comparison{Ranked: []multichain.Quote{ranked}}
	if got, ok := c.QuoteFor(ranked.ListingID); !ok || got.ListingID != ranked.ListingID {
		t.Fatalf("QuoteFor(ranked) = %v, %v, want the ranked quote", got, ok)
	}
	if _, ok := c.QuoteFor(multichain.ListingIDFrom(uuid.New())); ok {
		t.Fatal("QuoteFor(unranked) found a quote, want none")
	}
}

func TestQuote_grossOutputPastUint64KeepsItsExactValueAndIsOversize(t *testing.T) {
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
	var got multichain.Quote
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.GrossOutput.Cmp(wide) != 0 {
		t.Fatalf("GrossOutput = %s, want %s", got.GrossOutput, wide)
	}
	if got.GrossOutput.IsUint64() || !got.Oversize() || !q.Oversize() {
		t.Fatal("a gross output past uint64 must stay exact and report Oversize")
	}
}
