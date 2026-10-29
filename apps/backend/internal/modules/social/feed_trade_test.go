package social_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
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

func TestFeedTrade_aProposalTradeWritesNoTradeRow(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	r.mustCreate(t, func(*events.ProposalCreated) {})
	if err := r.passed(t); err != nil {
		t.Fatal(err)
	}
	if err := r.deliverTrade(t, r.confirmed("buy", "proposal")); err != nil {
		t.Fatal(err)
	}
	if rows := r.tradeRows(t); len(rows) != 0 {
		t.Fatalf("trade rows = %+v, want none", rows)
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

func TestFeedTrade_returnsWhatItCannotRead(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	e := r.confirmed("buy", "proposal")
	e.OutMint = marketfake.TSLAx().Mint.Address()
	if _, err := r.feed.FetchTrade(t.Context(), e); err == nil {
		t.Fatal("FetchTrade of an uncatalogued mint = nil, want an error")
	}
	for name, ddl := range map[string]string{
		"lookup": "ALTER TABLE feed_objects RENAME TO feed_objects_gone",
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
