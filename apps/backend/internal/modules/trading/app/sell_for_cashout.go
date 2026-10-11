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
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	cashOutSlippageBps    = 100
	bridgeBackEstimateBps = 75
	bridgeBackEstimateOf  = 100_000
)

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
	Routes   *RouteExec
	EVMSales EVMSales
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
	Listing   *storedListing      `json:"listing,omitempty"`
}

type storedListing struct {
	ID           multichain.ListingID `json:"id"`
	StockID      multichain.StockID   `json:"stock_id"`
	Network      chain.Network        `json:"network"`
	TokenAddress string               `json:"token_address"`
	Venue        multichain.Venue     `json:"venue"`
}

func cashOutBlocked(cabal ids.CabalID, src domain.Source, code errs.Code) events.TradeBlocked {
	return events.TradeBlocked{
		V: 1, CabalID: cabal.UUID(), Action: string(domain.ActionSell), Code: code,
		Source: events.TradeSource{Kind: string(src.Kind), ID: src.ID},
	}
}

func (h *SellForCashOutHandler) HandleDelivery(
	ctx context.Context, d bus.Delivery, cmd SellForCashOut, heartbeat func(),
) error {
	recorded, err := sqlc.New(h.d.Reads).DeliveryRecorded(ctx, sqlc.DeliveryRecordedParams{
		Handler: d.Handler, EventID: d.EventID.UUID(),
	})
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, "trading.SellForCashOut.HandleDelivery")
	}
	if recorded {
		return nil
	}
	cause := h.Handle(ctx, cmd, heartbeat)
	if cause == nil {
		return h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
			_, err := d.Record(ctx, tx)
			return err
		})
	}
	code := errs.CodeOf(cause)
	if errs.Retryable(code) && !d.Final {
		return cause
	}
	gaveUp := false
	err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		gaveUp = false
		inserted, err := d.RecordAs(ctx, tx, string(code))
		if err != nil || !inserted {
			return err
		}
		gaveUp = true
		return tx.Events.Append(ctx, cashOutBlocked(cmd.CabalID, cmd.Source, code))
	})
	if err != nil || !gaveUp {
		return err
	}
	observability.Alert(ctx, observability.TradingCashOutSellBlocked,
		slog.String("job_id", cmd.Source.ID.String()), slog.String("cabal_id", cmd.CabalID.String()),
		slog.String("code", string(code)), slog.Any("err", cause), slog.Bool("final", d.Final))
	return nil
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
		if err := h.sell(ctx, req, leg.Listing, heartbeat); err != nil {
			return err
		}
	}
	return nil
}

func (h *SellForCashOutHandler) sell(
	ctx context.Context, req SwapRequest, listing *multichain.Listing, heartbeat func(),
) error {
	if listing != nil {
		req.InMint.Address = chain.SolanaAddress(evmAsset(listing.Network, listing.TokenAddress))
		req.Route = saleRoute(*listing, req.QuoteOutAmount)
	}
	tried, err := sqlc.New(h.d.Reads).HasSwapForMint(ctx, sqlc.HasSwapForMintParams{
		SourceKind: string(req.Source.Kind), SourceID: req.Source.ID, InMint: string(req.InMint.Address),
	})
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, "trading.SellForCashOut.sell")
	}
	if tried {
		return nil
	}
	if listing != nil {
		_, err = h.d.Routes.Trade(ctx, req, *listing, heartbeat)
		return err
	}
	_, err = h.d.Layer.Run(ctx, req, heartbeat)
	return err
}

func saleRoute(l multichain.Listing, net uint64) *multichain.Comparison {
	gross, _ := money.MulDiv(net, bridgeBackEstimateOf, bridgeBackEstimateOf-bridgeBackEstimateBps)
	return &multichain.Comparison{
		StockID: l.StockID, Side: multichain.SideSell, Winner: l.ID,
		Ranked: []multichain.Quote{{
			ListingID: l.ID, Network: l.Network, Venue: l.Venue, Side: multichain.SideSell,
			Costs:     []multichain.Cost{{Kind: multichain.CostBridge, Micros: money.MicrosFromUint64(gross - net)}},
			NetMicros: money.MicrosFromUint64(net),
		}},
	}
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
		if l.Listing != nil {
			stored[i].Listing = &storedListing{
				ID:           l.Listing.ID,
				StockID:      l.Listing.StockID,
				Network:      l.Listing.Network,
				TokenAddress: l.Listing.TokenAddress,
				Venue:        l.Listing.Venue,
			}
		}
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInvalidInput, "trading.SellForCashOut.plan")
	}
	err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		n, err := sqlc.New(tx.Queries()).InsertCashoutSellPlan(ctx, sqlc.InsertCashoutSellPlanParams{
			JobID: cmd.Source.ID, CabalID: cmd.CabalID.UUID(), Legs: raw, CreatedAt: h.d.Clock.Now(),
		})
		if err != nil || n == 0 || len(legs) > 0 {
			return err
		}
		return tx.Events.Append(ctx, cashOutBlocked(cmd.CabalID, cmd.Source, errs.CodeAssetUntradable))
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
		if s.Listing != nil {
			legs[i].Listing = &multichain.Listing{
				ID:           s.Listing.ID,
				StockID:      s.Listing.StockID,
				Symbol:       s.Symbol,
				Network:      s.Listing.Network,
				TokenAddress: s.Listing.TokenAddress,
				Decimals:     s.Decimals,
				Venue:        s.Listing.Venue,
			}
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
		lot, err := h.value(ctx, held)
		if err != nil {
			return nil, err
		}
		if lot.QuoteUSDC > 0 {
			lots = append(lots, lot)
		}
	}
	return lots, nil
}

func (h *SellForCashOutHandler) value(ctx context.Context, held Holding) (domain.Lot, error) {
	lot := domain.Lot{Mint: held.Mint, Symbol: held.Symbol, Units: held.Units}
	if isEVMAsset(string(held.Mint.Address)) {
		sale, err := h.d.EVMSales.Sale(ctx, string(held.Mint.Address), held.Units)
		if err != nil {
			return domain.Lot{}, err
		}
		fee, err := money.MulDiv(sale.ProceedsMicros, bridgeBackEstimateBps, bridgeBackEstimateOf)
		lot.QuoteUSDC, lot.Listing, lot.Symbol = sale.ProceedsMicros-fee, &sale.Listing, sale.Listing.Symbol
		return lot, err
	}
	q, err := h.d.Venue.Quote(ctx, QuoteSpec{InMint: held.Mint, OutMint: h.d.USDC, InAmount: held.Units})
	if err != nil || !q.Routable {
		return domain.Lot{}, err
	}
	lot.QuoteUSDC = q.OutAmount
	return lot, nil
}
