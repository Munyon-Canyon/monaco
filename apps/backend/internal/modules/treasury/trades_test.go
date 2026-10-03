package treasury_test

import (
	"context"
	"math"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const swapSig = chain.Signature(
	"5VERv8NMvzbJMEkV8xnrLkEaWRtSz9CosKDYjCJjBRnbJLgp8uirBgmQpjKhoR4tjF3ZpRzrFmBV6UjKdiSZkQUW")

func confirmedAt() time.Time { return time.Date(2026, 3, 1, 12, 0, 5, 0, time.UTC) }

func (f fixture) confirm(t *testing.T, e events.TradeConfirmed) error {
	t.Helper()
	h := adapters.Trades{Ledger: f.ledger}
	return f.do(func(ctx context.Context, tx db.Tx) error { return h.Handle(ctx, tx, e, confirmedAt()) })
}

func (f fixture) funded(t *testing.T) ids.CabalID {
	t.Helper()
	const micros = 100_000_000
	cabal := f.cabal(t)
	u, c, err := f.fund(f.user(t), cabal, micros, micros, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	return cabal
}

func buy(cabal ids.CabalID, swap uuid.UUID, micros, units, fee uint64) events.TradeConfirmed {
	return events.TradeConfirmed{
		V: 1, SwapID: swap, CabalID: cabal.UUID(), Action: "buy", Symbol: "AAPLx",
		InMint: usdcMint, InAmount: micros, OutMint: chain.SolanaAddress(aapl), OutAmount: units,
		USDCMicros: money.MicrosFromUint64(micros), FeeMicros: money.MicrosFromUint64(fee),
		TxSignature: swapSig, ConfirmedAt: confirmedAt(),
	}
}

func sell(cabal ids.CabalID, swap uuid.UUID, units, micros uint64) events.TradeConfirmed {
	return events.TradeConfirmed{
		V: 1, SwapID: swap, CabalID: cabal.UUID(), Action: "sell", Symbol: "AAPLx",
		InMint: chain.SolanaAddress(aapl), InAmount: units, OutMint: usdcMint, OutAmount: micros,
		USDCMicros: money.MicrosFromUint64(micros), TxSignature: swapSig, ConfirmedAt: confirmedAt(),
	}
}

type header struct {
	Kind, Status, Signature string
	SwapID                  uuid.UUID
	CreatedAt               time.Time
	Entries                 []string
}

func (f fixture) swapHeaders(t *testing.T) []header {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `
		SELECT c.kind, c.status, c.tx_signature, c.swap_id, c.created_at,
			array_agg(e.account || ' ' || e.asset || ' ' || e.amount ORDER BY e.seq)
		FROM cabal_txns c JOIN cabal_txn_entries e ON e.txn_id = c.id
		WHERE c.kind = 'swap' GROUP BY c.id ORDER BY c.seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []header
	for rows.Next() {
		var h header
		if err := rows.Scan(&h.Kind, &h.Status, &h.Signature, &h.SwapID, &h.CreatedAt, &h.Entries); err != nil {
			t.Fatal(err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func wantPositions(t *testing.T, f fixture, want ...position) {
	t.Helper()
	if got := f.cabalPositions(t); !slices.Equal(got, want) {
		t.Fatalf("cabal positions = %+v, want %+v", got, want)
	}
	if drift := f.drift(t); len(drift) != 0 {
		t.Fatalf("drift = %q", drift)
	}
}

func TestTradesConsumer_BuyPostsBalancedHeader(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	swap := f.ids.NewV7()
	if err := f.confirm(t, buy(cabal, swap, 60_000_000, 3_000_000, 0)); err != nil {
		t.Fatal(err)
	}
	want := header{
		Kind: "swap", Status: "settled", Signature: string(swapSig), SwapID: swap, CreatedAt: confirmedAt(),
		Entries: []string{
			"treasury " + usdcMint + " -60000000", "venue " + usdcMint + " 60000000",
			"venue " + string(aapl) + " -3000000", "treasury " + string(aapl) + " 3000000",
		},
	}
	got := f.swapHeaders(t)
	if len(got) == 1 {
		got[0].CreatedAt = got[0].CreatedAt.UTC()
	}
	if len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Fatalf("swap headers = %+v, want one %+v", got, want)
	}
	wantPositions(t, f,
		position{Asset: usdcMint, Units: "40000000", Cost: "40000000"},
		position{Asset: string(aapl), Units: "3000000", Cost: "60000000"})
}

func TestTradesConsumer_SellReducesCostBasis(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	if err := f.confirm(t, buy(cabal, f.ids.NewV7(), 60_000_000, 3_000_000, 0)); err != nil {
		t.Fatal(err)
	}
	if err := f.confirm(t, sell(cabal, f.ids.NewV7(), 1_000_000, 25_000_000)); err != nil {
		t.Fatal(err)
	}
	wantPositions(t, f,
		position{Asset: usdcMint, Units: "65000000", Cost: "65000000"},
		position{Asset: string(aapl), Units: "2000000", Cost: "40000000"})
	if err := f.confirm(t, sell(cabal, f.ids.NewV7(), 1_999_999, 30_000_000)); err != nil {
		t.Fatal(err)
	}
	wantPositions(t, f,
		position{Asset: usdcMint, Units: "95000000", Cost: "95000000"},
		position{Asset: string(aapl), Units: "1", Cost: "20"})
	if got := len(f.swapHeaders(t)); got != 3 {
		t.Fatalf("swap headers = %d, want 3", got)
	}
}

func TestTradesConsumer_feeGoesFromTreasuryToFeesAndIntoCostBasis(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	if err := f.confirm(t, buy(cabal, f.ids.NewV7(), 60_000_000, 3_000_000, 250_000)); err != nil {
		t.Fatal(err)
	}
	got := f.swapHeaders(t)
	fee := []string{"treasury " + usdcMint + " -250000", "fees " + usdcMint + " 250000"}
	if len(got) != 1 || !slices.Equal(got[0].Entries[4:], fee) {
		t.Fatalf("swap headers = %+v, want the fee legs %q last", got, fee)
	}
	wantPositions(t, f,
		position{Asset: usdcMint, Units: "39750000", Cost: "39750000"},
		position{Asset: string(aapl), Units: "3000000", Cost: "60250000"})
}

func TestTradesConsumer_aSecondConfirmationOfOneSwapIsAlreadyHandled(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	e := buy(cabal, f.ids.NewV7(), 60_000_000, 3_000_000, 0)
	for range 2 {
		if err := f.confirm(t, e); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(f.swapHeaders(t)); got != 1 {
		t.Fatalf("swap headers = %d, want 1", got)
	}
	wantPositions(t, f,
		position{Asset: usdcMint, Units: "40000000", Cost: "40000000"},
		position{Asset: string(aapl), Units: "3000000", Cost: "60000000"})
}

func TestTradesConsumer_refusesAnAmountPastInt64AndAnOverdraftWithoutWriting(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	before := f.count(t, "cabal_txns")
	err := f.confirm(t, buy(cabal, f.ids.NewV7(), math.MaxInt64+1, 1, 0))
	wantCode(t, err, errs.CodeInvalidInput)
	if err := f.confirm(t, sell(cabal, f.ids.NewV7(), 1, 1)); err == nil {
		t.Fatal("selling an asset the cabal does not hold succeeded")
	}
	if got := f.count(t, "cabal_txns"); got != before {
		t.Fatalf("cabal_txns = %d, want %d", got, before)
	}
}

func TestTradesConsumer_aCanceledContextWritesNothing(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	ctx, cancel := context.WithCancel(f.ctx())
	err := f.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		cancel()
		return adapters.Trades{Ledger: f.ledger}.Handle(ctx, tx, buy(cabal, f.ids.NewV7(), 1, 1, 0), confirmedAt())
	})
	wantCode(t, err, errs.CodeDBUnavailable)
	if got := len(f.swapHeaders(t)); got != 0 {
		t.Fatalf("swap headers = %d, want 0", got)
	}
}
