package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const cashOutSlippageBps = 100

type SellForCashOut struct {
	CabalID    ids.CabalID
	Source     domain.Source
	USDCNeeded money.Micros
}

type SellForCashOutDeps struct {
	Layer    *SwapLayer
	UoW      *db.UnitOfWork
	Reads    sqlc.DBTX
	Clock    clock.Clock
	Venue    Venue
	Holdings Holdings
	Wallets  Wallets
	USDC     chain.Mint
}

type SellForCashOutHandler struct {
	d SellForCashOutDeps
}

func NewSellForCashOutHandler(d SellForCashOutDeps) *SellForCashOutHandler {
	return &SellForCashOutHandler{d: d}
}

type storedLeg struct {
	Mint      chain.SolanaAddress `json:"mint"`
	Decimals  uint8               `json:"decimals"`
	Symbol    string              `json:"symbol"`
	Units     uint64              `json:"units"`
	QuoteUSDC uint64              `json:"quote_usdc"`
}

func (h *SellForCashOutHandler) Handle(ctx context.Context, cmd SellForCashOut, heartbeat func()) error {
	if cmd.Source.Kind != domain.SourceCashout {
		return errs.New(
			errs.CodeInvalidInput,
			"trading.SellForCashOut",
			slog.String("source_kind", string(cmd.Source.Kind)),
		)
	}
	if cmd.USDCNeeded.IsZero() {
		return nil
	}
	legs, err := h.plan(ctx, cmd)
	if err != nil {
		return err
	}
	faultpoint.Hit(ctx, faultpoint.AfterSellRequest)
	wallet, err := h.d.Wallets.TreasuryWallet(ctx, cmd.CabalID)
	if err != nil {
		return err
	}
	for _, leg := range legs {
		req := SwapRequest{
			Source: cmd.Source, CabalID: cmd.CabalID, TreasuryWallet: wallet, Action: domain.ActionSell,
			Symbol: leg.Symbol, InMint: leg.Mint, OutMint: h.d.USDC, InAmount: leg.Units, QuoteOutAmount: leg.QuoteUSDC,
			SlippageBps: cashOutSlippageBps, SourceBatchSize: len(legs),
		}
		if err := h.sell(ctx, req, heartbeat); err != nil {
			return err
		}
	}
	return nil
}

func (h *SellForCashOutHandler) sell(ctx context.Context, req SwapRequest, heartbeat func()) error {
	tried, err := sqlc.New(h.d.Reads).HasSwapForMint(ctx, sqlc.HasSwapForMintParams{
		SourceKind: string(req.Source.Kind), SourceID: req.Source.ID, InMint: string(req.InMint.Address),
	})
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, "trading.SellForCashOut.sell")
	}
	if tried {
		return nil
	}
	_, err = h.d.Layer.Run(ctx, req, heartbeat)
	return err
}

func (h *SellForCashOutHandler) plan(ctx context.Context, cmd SellForCashOut) ([]domain.Lot, error) {
	if legs, ok, err := h.stored(ctx, cmd.Source); ok || err != nil {
		return legs, err
	}
	lots, err := h.quote(ctx, cmd.CabalID)
	if err != nil {
		return nil, err
	}
	legs, err := domain.PlanCashOutSell(lots, cmd.USDCNeeded)
	if err != nil {
		return nil, err
	}
	stored := make([]storedLeg, len(legs))
	for i, l := range legs {
		stored[i] = storedLeg{
			Mint: l.Mint.Address, Decimals: l.Mint.Decimals, Symbol: l.Symbol, Units: l.Units, QuoteUSDC: l.QuoteUSDC,
		}
	}
	raw, _ := json.Marshal(stored)
	err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		n, err := sqlc.New(tx.Queries()).InsertCashoutSellPlan(ctx, sqlc.InsertCashoutSellPlanParams{
			JobID: cmd.Source.ID, CabalID: cmd.CabalID.UUID(), Legs: raw, CreatedAt: h.d.Clock.Now(),
		})
		if err != nil || n == 0 || len(legs) > 0 {
			return err
		}
		return tx.Events.Append(ctx, events.TradeBlocked{
			V: 1, CabalID: cmd.CabalID.UUID(), Action: string(domain.ActionSell), Code: errs.CodeAssetUntradable,
			Source: events.TradeSource{Kind: string(cmd.Source.Kind), ID: cmd.Source.ID},
		})
	})
	if err != nil {
		return nil, err
	}
	legs, _, err = h.stored(ctx, cmd.Source)
	return legs, err
}

func (h *SellForCashOutHandler) stored(ctx context.Context, src domain.Source) ([]domain.Lot, bool, error) {
	const op = "trading.SellForCashOut.stored"
	raw, err := sqlc.New(h.d.Reads).CashoutSellPlan(ctx, src.ID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, false, nil
	case err != nil:
		return nil, false, errs.Wrap(err, errs.CodeInternal, op)
	}
	var stored []storedLeg
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return nil, false, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	legs := make([]domain.Lot, len(stored))
	for i, s := range stored {
		legs[i] = domain.Lot{
			Mint: chain.Mint{
				Address:  s.Mint,
				Decimals: s.Decimals,
			},
			Symbol:    s.Symbol,
			Units:     s.Units,
			QuoteUSDC: s.QuoteUSDC,
		}
	}
	return legs, true, nil
}

func (h *SellForCashOutHandler) quote(ctx context.Context, cabal ids.CabalID) ([]domain.Lot, error) {
	holdings, err := h.d.Holdings.Positions(ctx, cabal)
	if err != nil {
		return nil, err
	}
	lots := make([]domain.Lot, 0, len(holdings))
	for _, held := range holdings {
		if held.Mint.Address == h.d.USDC.Address || held.Units == 0 {
			continue
		}
		q, err := h.d.Venue.Quote(ctx, QuoteSpec{InMint: held.Mint, OutMint: h.d.USDC, InAmount: held.Units})
		if err != nil {
			return nil, err
		}
		if q.Routable {
			lots = append(
				lots,
				domain.Lot{Mint: held.Mint, Symbol: held.Symbol, Units: held.Units, QuoteUSDC: q.OutAmount},
			)
		}
	}
	return lots, nil
}
