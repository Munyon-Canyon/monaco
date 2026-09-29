package jupiterfake_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/jupiterfake"
)

type venue interface {
	Order(ctx context.Context, spec jupiter.OrderSpec) (jupiter.Order, error)
	Quote(ctx context.Context, spec jupiter.QuoteSpec) (jupiter.Quote, error)
	Execute(ctx context.Context, requestID string, signed []byte) (jupiter.ExecuteResult, error)
	ExecuteUntilTerminal(ctx context.Context, requestID string, signed []byte) (jupiter.ExecuteResult, error)
}

type priceSource interface {
	Prices(ctx context.Context, mints []jupiter.Mint) (map[jupiter.Mint]jupiter.Price, error)
}

var (
	_ venue       = (*jupiter.Client)(nil)
	_ venue       = (*jupiterfake.Venue)(nil)
	_ priceSource = (*jupiter.Client)(nil)
	_ priceSource = (*jupiterfake.PriceSource)(nil)
)

func usdc() jupiter.Mint {
	return jupiter.Mint{Address: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", Decimals: 6}
}

func aaplx() jupiter.Mint {
	return jupiter.Mint{Address: "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp", Decimals: 8}
}

func TestVenue_ordersAndQuotesArePerPair(t *testing.T) {
	t.Parallel()
	var v jupiterfake.Venue
	buy := jupiter.Order{RequestID: "req-buy", Transaction: []byte("tx"), InMint: usdc(), OutMint: aaplx()}
	v.SetOrder(usdc(), aaplx(), buy)
	v.SetQuote(usdc(), aaplx(), jupiter.Quote{Routable: true, PriceImpactBps: 7})

	got, err := v.Order(t.Context(), jupiter.OrderSpec{In: usdc(), Out: aaplx()})
	if err != nil || !reflect.DeepEqual(got, buy) {
		t.Fatalf("Order = %+v, %v, want %+v", got, err, buy)
	}
	if _, err := v.Order(
		t.Context(),
		jupiter.OrderSpec{In: aaplx(), Out: usdc()},
	); errs.CodeOf(
		err,
	) != errs.CodeJupiterRejected {
		t.Fatalf("unscripted Order err = %v, want jupiter_rejected", err)
	}
	if q, err := v.Quote(
		t.Context(),
		jupiter.QuoteSpec{In: usdc(), Out: aaplx()},
	); err != nil ||
		q.PriceImpactBps != 7 {
		t.Fatalf("Quote = %+v, %v", q, err)
	}
	if q, err := v.Quote(t.Context(), jupiter.QuoteSpec{In: aaplx(), Out: usdc()}); err != nil || q.Routable {
		t.Fatalf("unscripted Quote = %+v, %v, want unroutable", q, err)
	}
}

func TestVenue_executeReplaysThePerRequestSequence(t *testing.T) {
	t.Parallel()
	var v jupiterfake.Venue
	pending := jupiter.ExecuteResult{Status: jupiter.StatusPending}
	done := jupiter.ExecuteResult{Status: jupiter.StatusSuccess, Signature: "sig-1", OutAmount: 42}
	v.SetExecute("req-1", pending, pending, done)

	got, err := v.ExecuteUntilTerminal(t.Context(), "req-1", []byte("signed"))
	if err != nil || got != done {
		t.Fatalf("ExecuteUntilTerminal = %+v, %v, want %+v", got, err, done)
	}
	if sent := v.Sent("req-1"); len(sent) != 3 || string(sent[2]) != "signed" {
		t.Fatalf("Sent = %q, want the signed bytes 3 times", sent)
	}
	if got, err := v.Execute(t.Context(), "req-1", nil); err != nil || got != done {
		t.Fatalf("Execute after the sequence = %+v, %v, want the last result again", got, err)
	}
	if got, err := v.Execute(t.Context(), "req-2", nil); err != nil || got.Status != jupiter.StatusSuccess ||
		got.Signature != "sig-req-2" {
		t.Fatalf("unscripted Execute = %+v, %v, want Success", got, err)
	}
}

func TestVenue_executeThatStaysPendingTimesOut(t *testing.T) {
	t.Parallel()
	var v jupiterfake.Venue
	v.SetExecute("req-1", jupiter.ExecuteResult{Status: jupiter.StatusPending})
	got, err := v.ExecuteUntilTerminal(t.Context(), "req-1", nil)
	if errs.CodeOf(err) != errs.CodeUpstreamTimeout || got.Status != jupiter.StatusPending {
		t.Fatalf("ExecuteUntilTerminal = %+v, %v, want Pending with upstream_timeout", got, err)
	}
}

func TestFaultsFailEachCall(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeJupiterUnavailable, "test")
	var v jupiterfake.Venue
	var p jupiterfake.PriceSource
	for _, op := range []string{"Order", "Quote", "Execute"} {
		v.FailOnce(op, down)
	}
	p.FailOnce("Prices", down)
	calls := map[string]error{}
	_, calls["Order"] = v.Order(t.Context(), jupiter.OrderSpec{})
	_, calls["Quote"] = v.Quote(t.Context(), jupiter.QuoteSpec{})
	_, calls["Execute"] = v.ExecuteUntilTerminal(t.Context(), "req-1", nil)
	_, calls["Prices"] = p.Prices(t.Context(), nil)
	for op, err := range calls {
		if errs.CodeOf(err) != errs.CodeJupiterUnavailable {
			t.Fatalf("%s err = %v, want the injected fault", op, err)
		}
	}
}

func TestPriceSource_returnsOnlyScriptedMints(t *testing.T) {
	t.Parallel()
	var p jupiterfake.PriceSource
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	price := jupiter.Price{Mint: aaplx(), USDMicros: money.MicrosFromUint64(212_345_678), ObservedAt: at}
	p.SetPrice(price)
	got, err := p.Prices(t.Context(), []jupiter.Mint{aaplx(), usdc()})
	if err != nil || len(got) != 1 || got[aaplx()] != price {
		t.Fatalf("Prices = %+v, %v, want only %+v", got, err, price)
	}
}
