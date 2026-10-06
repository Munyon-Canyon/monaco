package social_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const usdcMint = chain.SolanaAddress("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")

func (r proposalRig) confirmed(side, source string) events.TradeConfirmed {
	e := events.TradeConfirmed{
		V: 1, SwapID: r.gen.NewV7(), CabalID: r.cabal, Source: events.TradeSource{Kind: source, ID: r.proposal},
		Action: side, Symbol: "AAPLx", USDCMicros: money.MicrosFromUint64(500_000_000), ConfirmedAt: r.now,
	}
	stock := marketfake.AAPLx().Mint.Address()
	if side == "sell" {
		e.InMint, e.InAmount, e.OutMint, e.OutAmount = stock, 250_000_000, usdcMint, 500_000_000
		return e
	}
	e.InMint, e.InAmount, e.OutMint, e.OutAmount = usdcMint, 500_000_000, stock, 250_000_000
	return e
}

func (r proposalRig) deliverTrade(t *testing.T, e events.TradeConfirmed) error {
	t.Helper()
	return r.deliver(t, func(ctx context.Context, tx db.Tx) error {
		card, err := r.feed.FetchTrade(ctx, e)
		if err != nil {
			return err
		}
		return r.feed.ApplyTrade(ctx, tx, e, card, r.clock.Now())
	})
}

type tradeRow struct {
	Title, Symbol, Actor string
	Asset                uuid.UUID
	Payload              json.RawMessage
}

func (r proposalRig) tradeRows(t *testing.T) []tradeRow {
	t.Helper()
	rows, err := r.pool.Query(t.Context(), `SELECT title, symbol, coalesce(actor_id::text, ''), asset_id, payload
		FROM feed_objects WHERE kind = 'trade' AND ref_type = 'swaps'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []tradeRow
	for rows.Next() {
		var row tradeRow
		if err := rows.Scan(&row.Title, &row.Symbol, &row.Actor, &row.Asset, &row.Payload); err != nil {
			t.Fatal(err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestFeedTrade_Confirmed_InsertsOnce(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	sub := testkit.SubscribeCore(t, r.conn, hintSubject)
	e := r.confirmed("buy", "proposal")
	for range 2 {
		if err := r.deliverTrade(t, e); err != nil {
			t.Fatal(err)
		}
	}
	rows := r.tradeRows(t)
	if len(rows) != 1 {
		t.Fatalf("trade items after a redelivery = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.Title != "Alpha bought $500 of AAPLx" || row.Symbol != "AAPLx" || row.Actor != "" ||
		row.Asset != marketfake.AAPLx().ID.UUID() {
		t.Fatalf("row = %+v", row)
	}
	var ref uuid.UUID
	err := r.pool.QueryRow(t.Context(), `SELECT ref_id FROM feed_objects WHERE kind = 'trade'`).Scan(&ref)
	if err != nil || ref != e.SwapID {
		t.Fatalf("ref_id = %s, %v, want swap %s", ref, err, e.SwapID)
	}
	wantHints(t, r.conn.Conn, sub, 2)
}

func TestFeedTrade_PayloadHoldsIntegersAndNoMint(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	e := r.confirmed("buy", "proposal")
	if err := r.deliverTrade(t, e); err != nil {
		t.Fatal(err)
	}
	row := r.tradeRows(t)[0]
	want := map[string]string{
		"cabal_name": `"Alpha"`, "symbol": `"AAPLx"`, "asset_name": `"Apple"`, "action": `"buy"`,
		"usdc_micros": `"500000000"`, "price_micros": `"200000000"`, "token_amount": `"250000000"`,
		"token_decimals": `8`, "proposal_id": `"` + r.proposal.String() + `"`,
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(row.Payload, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("payload = %s, want %v", row.Payload, want)
	}
	for key, w := range want {
		if string(got[key]) != w {
			t.Errorf("payload[%s] = %s, want %s", key, got[key], w)
		}
	}
	payload, err := feed.ParsePayload(row.Payload)
	if err != nil {
		t.Fatal(err)
	}
	text := row.Title + "|" + feed.RenderDetail(feed.KindTrade, payload)
	if strings.Contains(text, string(e.OutMint)) || strings.Contains(text, string(e.InMint)) {
		t.Fatalf("title and detail %q hold a mint", text)
	}
	if got := feed.RenderDetail(feed.KindTrade, payload); got != "Filled at $200.00" {
		t.Fatalf("detail = %q", got)
	}
}

func TestFeedTrade_SellReadsTheStockLegAndSaysSold(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	if err := r.deliverTrade(t, r.confirmed("sell", "proposal")); err != nil {
		t.Fatal(err)
	}
	row := r.tradeRows(t)[0]
	payload, err := feed.ParsePayload(row.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if row.Title != "Alpha sold $500 of AAPLx" || payload.TokenAmount != 250_000_000 ||
		payload.PriceMicros.Uint64() != 200_000_000 {
		t.Fatalf("row = %+v payload = %+v", row, payload)
	}
}

func TestFeedTrade_TheCatalogNamesASymbolTheEventLacks(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	e := r.confirmed("buy", "proposal")
	e.Symbol = ""
	if err := r.deliverTrade(t, e); err != nil {
		t.Fatal(err)
	}
	if row := r.tradeRows(t)[0]; row.Symbol != "AAPLx" || row.Title != "Alpha bought $500 of AAPLx" {
		t.Fatalf("row = %+v", row)
	}
}

func TestFeedTrade_Cashout_NoItem(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	sub := testkit.SubscribeCore(t, r.conn, hintSubject)
	for _, source := range []string{"cashout", "agent"} {
		if err := r.deliverTrade(t, r.confirmed("sell", source)); err != nil {
			t.Fatalf("%s: %v", source, err)
		}
	}
	if rows := r.tradeRows(t); len(rows) != 0 {
		t.Fatalf("items = %+v, want none", rows)
	}
	wantHints(t, r.conn.Conn, sub, 0)
}

func TestFeedTrade_aTradeBeforeItsCabalNaks(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	r.cabal = r.gen.NewV7()
	if got := errs.CodeOf(r.deliverTrade(t, r.confirmed("buy", "proposal"))); got != errs.CodeFeedItemPending {
		t.Fatalf("trade before cabal = %s, want %s", got, errs.CodeFeedItemPending)
	}
}

func TestFeedTrade_returnsWhatItCannotRead(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	e := r.confirmed("buy", "proposal")
	e.OutMint = marketfake.TSLAx().Mint.Address()
	if _, err := r.feed.FetchTrade(t.Context(), e); err == nil {
		t.Fatal("FetchTrade of an uncatalogued mint = nil, want an error")
	}
	for name, ddl := range map[string]string{
		"cabal lookup": "ALTER TABLE feed_cabals RENAME TO feed_cabals_gone",
		"insert":       "ALTER TABLE feed_objects RENAME TO feed_objects_gone",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newProposalRig(t)
			if _, err := r.pool.Exec(t.Context(), ddl); err != nil {
				t.Fatal(err)
			}
			err := r.deliverTrade(t, r.confirmed("buy", "proposal"))
			if err == nil || errs.CodeOf(err) == errs.CodeFeedItemPending {
				t.Fatalf("trade = %v, want a database failure", err)
			}
		})
	}
}

func TestFeedTrade_aCashoutNeverReadsTheCatalog(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	e := r.confirmed("sell", "cashout")
	e.InMint = marketfake.TSLAx().Mint.Address()
	card, err := r.feed.FetchTrade(t.Context(), e)
	if err != nil || card != (app.AssetCard{}) {
		t.Fatalf("FetchTrade = %+v, %v, want an empty card", card, err)
	}
}
