package app

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type ExecuteTrade struct {
	ProposalID     ids.ProposalID
	CabalID        ids.CabalID
	Action         domain.Action
	Symbol         string
	Mint           chain.SolanaAddress
	USDCMicros     money.Micros
	TokenAmount    uint64
	QuoteOutAmount uint64
}

func (c ExecuteTrade) source() domain.Source {
	return domain.Source{Kind: domain.SourceProposal, ID: c.ProposalID.UUID()}
}

func (c ExecuteTrade) amount() uint64 {
	if c.Action == domain.ActionBuy {
		return c.USDCMicros.Uint64()
	}
	return c.TokenAmount
}

type ExecuteTradeDeps struct {
	Layer *SwapLayer
	UoW   *db.UnitOfWork
	Reads sqlc.DBTX
	Venue Venue
	Ports EnginePorts
	USDC  chain.Mint
}

type ExecuteTradeHandler struct {
	d ExecuteTradeDeps
}

func NewExecuteTradeHandler(d ExecuteTradeDeps) *ExecuteTradeHandler {
	return &ExecuteTradeHandler{d: d}
}

type refusal struct {
	code       errs.Code
	have, need uint64
}

func (r refusal) refused() bool { return r.code != "" }

func (h *ExecuteTradeHandler) Handle(ctx context.Context, d bus.Delivery, cmd ExecuteTrade, heartbeat func()) error {
	if done, err := h.claimed(ctx, d, cmd.source()); done || err != nil {
		return err
	}
	req, refused, err := h.check(ctx, cmd)
	switch {
	case err != nil:
		return err
	case refused.refused():
		return h.block(ctx, d, cmd, refused)
	}
	view, err := h.d.Layer.Run(ctx, req, heartbeat)
	if err != nil || view.Status == domain.StatusCreated {
		return err
	}
	return h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		_, err := d.Record(ctx, tx)
		return err
	})
}

func (h *ExecuteTradeHandler) claimed(ctx context.Context, d bus.Delivery, src domain.Source) (bool, error) {
	recorded, err := sqlc.New(h.d.Reads).DeliveryRecorded(ctx, sqlc.DeliveryRecordedParams{
		Handler: d.Handler, EventID: d.EventID.UUID(),
	})
	if err != nil {
		return false, errs.Wrap(err, errs.CodeInternal, "trading.ExecuteTrade.claimed")
	}
	if recorded {
		return true, nil
	}
	latest, found, err := NewQueries(h.d.Reads).LatestBySource(ctx, src)
	return found && latest.Status != domain.StatusFailed, err
}

func (h *ExecuteTradeHandler) check(ctx context.Context, cmd ExecuteTrade) (SwapRequest, refusal, error) {
	untradable := refusal{code: errs.CodeAssetUntradable}
	asset, err := h.d.Ports.Catalog.AssetBySymbol(ctx, cmd.Symbol)
	switch {
	case errs.CodeOf(err) == errs.CodeAssetNotFound:
		return SwapRequest{}, untradable, nil
	case err != nil:
		return SwapRequest{}, refusal{}, err
	case !asset.Tradable() || asset.Mint.Address() != cmd.Mint:
		return SwapRequest{}, untradable, nil
	}
	wallet, err := h.d.Ports.Cabals.TreasuryWallet(ctx, cmd.CabalID)
	if err != nil {
		return SwapRequest{}, refusal{}, err
	}
	req := h.request(cmd, asset, wallet)
	for _, next := range []func(context.Context, ExecuteTrade, *SwapRequest) (refusal, error){
		h.funds, h.price, h.paused,
	} {
		if refused, err := next(ctx, cmd, &req); refused.refused() || err != nil {
			return SwapRequest{}, refused, err
		}
	}
	return req, refusal{}, nil
}

func (h *ExecuteTradeHandler) request(cmd ExecuteTrade, asset market.Asset, w cabalport.TreasuryWallet) SwapRequest {
	token := chain.Mint{Address: cmd.Mint, Decimals: asset.Decimals}
	req := SwapRequest{
		Source: cmd.source(), CabalID: cmd.CabalID, Action: cmd.Action, Symbol: cmd.Symbol,
		TreasuryWallet: TreasuryWallet{PrivyWalletID: w.PrivyWalletID, Address: w.Address},
		InMint:         h.d.USDC, OutMint: token, InAmount: cmd.amount(), QuoteOutAmount: cmd.QuoteOutAmount,
		SourceBatchSize: 1,
	}
	if cmd.Action == domain.ActionSell {
		req.InMint, req.OutMint = token, h.d.USDC
	}
	return req
}

func (h *ExecuteTradeHandler) funds(ctx context.Context, cmd ExecuteTrade, req *SwapRequest) (refusal, error) {
	have, err := h.d.Ports.Balances.TokenBalance(ctx, req.TreasuryWallet.Address, req.InMint)
	if err != nil {
		return refusal{}, err
	}
	need := req.InAmount
	if cmd.Action == domain.ActionBuy {
		cfg, err := h.d.Ports.Balances.MintConfig(ctx, cmd.Mint)
		if err != nil {
			return refusal{}, err
		}
		headroom := domain.FeeHeadroom(req.InAmount, cfg.TransferFeeBps, cfg.MaxFee.Uint64())
		total, err := cmd.USDCMicros.Add(money.MicrosFromUint64(headroom))
		if err != nil {
			return refusal{}, err
		}
		need = total.Uint64()
	}
	if have.Uint64() < need {
		return refusal{code: errs.CodeInsufficientFunds, have: have.Uint64(), need: need}, nil
	}
	return refusal{}, nil
}

func (h *ExecuteTradeHandler) price(ctx context.Context, cmd ExecuteTrade, req *SwapRequest) (refusal, error) {
	bps, err := h.d.Ports.Cabals.SlippageBps(ctx, cmd.CabalID)
	if err != nil {
		return refusal{}, err
	}
	slippage := domain.SlippageOf(bps)
	req.SlippageBps = slippage.Bps()
	quote, err := h.d.Venue.Quote(ctx, QuoteSpec{InMint: req.InMint, OutMint: req.OutMint, InAmount: req.InAmount})
	if err != nil {
		return refusal{}, err
	}
	if !quote.Routable {
		return refusal{code: errs.CodeNoRoute}, nil
	}
	if floor := slippage.MinOut(cmd.QuoteOutAmount); quote.OutAmount < floor {
		return refusal{code: errs.CodeSlippageExceeded, have: quote.OutAmount, need: floor}, nil
	}
	return refusal{}, nil
}

func (h *ExecuteTradeHandler) paused(ctx context.Context, cmd ExecuteTrade, _ *SwapRequest) (refusal, error) {
	status, err := h.d.Ports.Cabals.Status(ctx, cmd.CabalID)
	if err != nil {
		return refusal{}, err
	}
	pause, err := h.d.Ports.Pauses.IsPaused(ctx, cmd.CabalID)
	if err != nil {
		return refusal{}, err
	}
	if status == cabalport.StatusBanned || pause.Paused {
		return refusal{code: errs.CodeCabalPaused}, nil
	}
	return refusal{}, nil
}

func (h *ExecuteTradeHandler) block(ctx context.Context, d bus.Delivery, cmd ExecuteTrade, r refusal) error {
	blocked := false
	err := h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		inserted, err := d.Record(ctx, tx)
		if err != nil || !inserted {
			return err
		}
		blocked = true
		return tx.Events.Append(ctx, events.TradeBlocked{
			V: 1, CabalID: cmd.CabalID.UUID(), Source: events.TradeSource{
				Kind: string(domain.SourceProposal),
				ID:   cmd.ProposalID.UUID(),
			}, SourceBatchSize: 1, Action: string(cmd.Action), Symbol: cmd.Symbol,
			Code: r.code, Have: r.have, Need: r.need,
		})
	})
	if err != nil || !blocked {
		return err
	}
	observability.Info(ctx, observability.TradingEngineBlocked,
		slog.String("proposal_id", cmd.ProposalID.String()), slog.String("cabal_id", cmd.CabalID.String()),
		slog.String("code", string(r.code)), slog.Uint64("have", r.have), slog.Uint64("need", r.need))
	return nil
}
