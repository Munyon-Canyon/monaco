package chain_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/adapters/chain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	platform "github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/jupiterfake"
)

func usdc() platform.Mint {
	return platform.Mint{Address: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", Decimals: 6}
}

func aaplx() platform.Mint {
	return platform.Mint{Address: "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp", Decimals: 8}
}

func jupiterMint(m platform.Mint) jupiter.Mint {
	return jupiter.Mint{Address: string(m.Address), Decimals: m.Decimals}
}

func TestVenue_quoteCarriesAmountsAndImpactAcrossThePort(t *testing.T) {
	t.Parallel()
	var fake jupiterfake.Venue
	fake.SetQuote(jupiterMint(usdc()), jupiterMint(aaplx()), jupiter.Quote{
		InAmount: money.NewBaseUnits(25_000_000, 6), OutAmount: money.NewBaseUnits(105_000_000, 8),
		PriceImpactBps: 12, Routable: true,
	})
	v := chain.NewVenue(&fake)
	got, err := v.Quote(t.Context(), app.QuoteSpec{InMint: usdc(), OutMint: aaplx(), InAmount: 25_000_000})
	want := app.Quote{InAmount: 25_000_000, OutAmount: 105_000_000, PriceImpactBps: 12, Routable: true}
	if err != nil || got != want {
		t.Fatalf("Quote = %+v, %v, want %+v", got, err, want)
	}
	none, err := v.Quote(t.Context(), app.QuoteSpec{InMint: aaplx(), OutMint: usdc(), InAmount: 1})
	if err != nil || none.Routable {
		t.Fatalf("Quote on an unscripted pair = %+v, %v, want not routable", none, err)
	}
	fake.Fail("Quote", errs.New(errs.CodeJupiterUnavailable, "test"))
	_, err = v.Quote(t.Context(), app.QuoteSpec{InMint: usdc(), OutMint: aaplx(), InAmount: 1})
	if errs.CodeOf(err) != errs.CodeJupiterUnavailable {
		t.Fatalf("Quote error = %v, want jupiter_unavailable", err)
	}
}

func TestVenue_orderCarriesTheRequestIDAndTransaction(t *testing.T) {
	t.Parallel()
	var fake jupiterfake.Venue
	fake.SetOrder(jupiterMint(usdc()), jupiterMint(aaplx()), jupiter.Order{
		RequestID: "req-1", Transaction: []byte{1, 2, 3},
	})
	v := chain.NewVenue(&fake)
	spec := app.OrderSpec{
		Taker: "taker", Payer: "relayer", InMint: usdc(), OutMint: aaplx(), InAmount: 25_000_000, SlippageBps: 100,
	}
	got, err := v.Order(t.Context(), spec)
	if err != nil || got.RequestID != "req-1" || string(got.Transaction) != "\x01\x02\x03" {
		t.Fatalf("Order = %+v, %v", got, err)
	}
	payerless := spec
	payerless.Payer = ""
	if _, err := v.Order(t.Context(), payerless); errs.CodeOf(err) != errs.CodeJupiterRejected {
		t.Fatalf("Order without a payer from a taker with no SOL = %v, want jupiter_rejected", err)
	}
	spec.InMint, spec.OutMint = aaplx(), usdc()
	_, err = v.Order(t.Context(), spec)
	if errs.CodeOf(err) != errs.CodeJupiterRejected {
		t.Fatalf("Order without a route = %v, want jupiter_rejected", err)
	}
}

func TestVenue_executeMapsEachJupiterStatusAndRefusesAnUnknownOne(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		in   jupiter.ExecuteResult
		want app.ExecuteResult
	}{
		"success": {
			jupiter.ExecuteResult{Status: jupiter.StatusSuccess, OutAmount: 7},
			app.ExecuteResult{Status: app.ExecuteSuccess, OutAmount: 7},
		},
		"failed": {
			jupiter.ExecuteResult{Status: jupiter.StatusFailed, ErrorCode: -1004},
			app.ExecuteResult{Status: app.ExecuteFailed, ErrorCode: -1004},
		},
		"pending": {
			jupiter.ExecuteResult{Status: jupiter.StatusPending},
			app.ExecuteResult{Status: app.ExecutePending},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var fake jupiterfake.Venue
			fake.SetExecute("req", tc.in)
			got, err := chain.NewVenue(&fake).ExecuteUntilTerminal(t.Context(), "req", []byte{1})
			if tc.in.Status == jupiter.StatusPending {
				if errs.CodeOf(err) != errs.CodeUpstreamTimeout {
					t.Fatalf("a pending result that never ends = %v, want upstream_timeout", err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("ExecuteUntilTerminal = %+v, %v, want %+v", got, err, tc.want)
			}
		})
	}
	var fake jupiterfake.Venue
	fake.SetExecute("req", jupiter.ExecuteResult{Status: 99})
	_, err := chain.NewVenue(&fake).ExecuteUntilTerminal(t.Context(), "req", []byte{1})
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("an unknown status = %v, want decode_failed", err)
	}
}
