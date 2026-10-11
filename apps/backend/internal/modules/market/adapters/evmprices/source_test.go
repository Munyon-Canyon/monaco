package evmprices_test

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/evmprices"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type quoter struct {
	gross map[string]*big.Int
	sides []multichain.Side
	sizes []money.Micros
}

func (q *quoter) Quote(
	_ context.Context, l multichain.Listing, side multichain.Side, amount money.Micros,
) (multichain.Quote, error) {
	q.sides, q.sizes = append(q.sides, side), append(q.sizes, amount)
	gross, ok := q.gross[l.TokenAddress]
	if !ok {
		return multichain.Quote{}, errs.New(errs.CodeUpstreamUnavailable, "test.Quote")
	}
	return multichain.Quote{
		ListingID: l.ID, InputMicros: amount, GrossOutput: gross, QuotedAt: time.Unix(1_800_000_000, 0),
		Costs: []multichain.Cost{{Kind: multichain.CostPoolFee, Micros: money.MicrosFromUint64(300_000)}},
	}, nil
}

func listing(t *testing.T, token string, decimals uint8, sharesPerToken string) multichain.Listing {
	t.Helper()
	spt, _ := new(big.Rat).SetString(sharesPerToken)
	var id multichain.ListingID
	if err := id.UnmarshalText([]byte("01928a6e-0000-7000-8000-00000000000" + token[len(token)-1:])); err != nil {
		t.Fatal(err)
	}
	return multichain.Listing{
		ID: id, Network: chain.NetworkEthereum, TokenAddress: token, Decimals: decimals, SharesPerToken: spt,
		Venue: multichain.VenueUniswap,
	}
}

func units(s string) *big.Int {
	v, _ := new(big.Int).SetString(s, 10)
	return v
}

func TestSource_pricesEachListingPerUnderlyingShareFromAReferenceBuyBeforeCosts(t *testing.T) {
	t.Parallel()
	ondo := listing(t, "0x1", 18, "1.25")
	xstock := listing(t, "0x2", 8, "1")
	dead := listing(t, "0x3", 18, "1")
	q := &quoter{gross: map[string]*big.Int{
		"0x1": units("320000000000000000"),
		"0x2": units("40000000"),
	}}

	got, err := evmprices.New(q).Prices(t.Context(), []multichain.Listing{ondo, xstock, dead})
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("err = %v, want the failed listing's code", err)
	}
	if len(got) != 2 || got[ondo.ID].PerShareMicros.Uint64() != 250_000_000 ||
		got[xstock.ID].PerShareMicros.Uint64() != 250_000_000 || !got[ondo.ID].AsOf.Equal(time.Unix(1_800_000_000, 0)) {
		t.Fatalf("Prices = %+v, want both priced listings at 250 USDC a share", got)
	}
	for i, side := range q.sides {
		if side != multichain.SideBuy || q.sizes[i].Uint64() != evmprices.ReferenceMicros {
			t.Fatalf("quote %d = %s %s, want a buy of the reference amount", i, side, q.sizes[i])
		}
	}
}

func TestPerShare_refusesAQuoteThatCannotPriceAShare(t *testing.T) {
	t.Parallel()
	l := listing(t, "0x1", 18, "1")
	for name, q := range map[string]multichain.Quote{
		"no output":   {InputMicros: money.MicrosFromUint64(1), GrossOutput: big.NewInt(0)},
		"nil output":  {InputMicros: money.MicrosFromUint64(1)},
		"below micro": {InputMicros: money.MicrosFromUint64(1), GrossOutput: units("2000000000000000000")},
	} {
		if _, err := evmprices.PerShare(q, l); errs.CodeOf(err) != errs.CodeDecodeFailed {
			t.Fatalf("%s: err = %v, want decode_failed", name, err)
		}
	}
}
