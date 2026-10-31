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
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
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
	Retry          *Retry
	Route          *multichain.Comparison
	Pin            chain.Network
	unrouted       bool
}

type Retry struct {
	Of             ids.SwapID
	SlippageBps    int32
	AtCurrentPrice bool
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
	Layer    *SwapLayer
	Routes   *RouteExec
	Listings Listings
	UoW      *db.UnitOfWork
	Reads    sqlc.DBTX
	Venue    Venue
	Ports    EnginePorts
	USDC     chain.Mint
	EVM      bool
	Router   multichain.Router
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
	if done, err := h.claimed(ctx, d, cmd); done || err != nil {
		return err
	}
	req, winner, refused, err := h.check(ctx, cmd)
	switch {
	case err != nil:
		return h.checkFailed(ctx, d, cmd, err)
	case refused.refused():
		return h.block(ctx, d, cmd, refused)
	}
	var view SwapView
	if winner.Network.IsEVM() {
		view, err = h.d.Routes.Trade(ctx, req, winner, heartbeat)
	} else {
		view, err = h.d.Layer.Run(ctx, req, heartbeat)
	}
	if err != nil || view.Status == domain.StatusCreated {
		return err
	}
	return h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		if view.Status == domain.StatusFailed {
			_, err := d.RecordAs(ctx, tx, string(failedOutcome(view.FailureCode)))
			return err
		}
		_, err := d.Record(ctx, tx)
		return err
	})
}

func failedOutcome(code domain.FailureCode) errs.Code {
	if code == domain.FailurePriceMoved {
		return errs.CodePriceMoved
	}
	return errs.CodeSwapFailed
}

func (h *ExecuteTradeHandler) checkFailed(ctx context.Context, d bus.Delivery, cmd ExecuteTrade, err error) error {
	code := errs.CodeOf(err)
	if errs.Retryable(code) && !d.Final {
		return err
	}
	observability.Alert(ctx, observability.TradingEngineBlocked,
		slog.String("proposal_id", cmd.ProposalID.String()), slog.String("cabal_id", cmd.CabalID.String()),
		slog.String("code", string(code)), slog.Uint64("have", 0), slog.Uint64("need", 0),
		slog.Any("err", err), slog.Bool("final", d.Final))
	return h.block(ctx, d, cmd, refusal{code: errs.CodeTradeNotStarted})
}

func (h *ExecuteTradeHandler) claimed(ctx context.Context, d bus.Delivery, cmd ExecuteTrade) (bool, error) {
	recorded, err := sqlc.New(h.d.Reads).DeliveryRecorded(ctx, sqlc.DeliveryRecordedParams{
		Handler: d.Handler, EventID: d.EventID.UUID(),
	})
	if err != nil {
		return false, errs.Wrap(err, errs.CodeInternal, "trading.ExecuteTrade.claimed")
	}
	if recorded {
		return true, nil
	}
	latest, found, err := NewQueries(h.d.Reads).LatestBySource(ctx, cmd.source())
	superseded := cmd.Retry != nil && latest.ID != cmd.Retry.Of
	if err != nil || !found || latest.Status == domain.StatusFailed && !superseded {
		return false, err
	}
	return true, h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		_, err := d.Record(ctx, tx)
		return err
	})
}

type tradeCheck func(context.Context, ExecuteTrade, *SwapRequest) (refusal, error)

func (h *ExecuteTradeHandler) check(
	ctx context.Context, cmd ExecuteTrade,
) (SwapRequest, multichain.Listing, refusal, error) {
	cmd, err := h.reroute(ctx, cmd)
	if code := errs.CodeOf(err); pinRefused(code) {
		return SwapRequest{}, multichain.Listing{}, refusal{code: code}, nil
	}
	if err != nil {
		return SwapRequest{}, multichain.Listing{}, refusal{}, err
	}
	for {
		req, winner, refused, err := h.prepare(ctx, cmd)
		if err != nil || refused.refused() {
			return req, winner, refused, err
		}
		next, fell, err := h.fallback(ctx, cmd, req, winner)
		if err != nil || !fell {
			return req, winner, refusal{}, err
		}
		cmd = next
	}
}

func (h *ExecuteTradeHandler) prepare(
	ctx context.Context, cmd ExecuteTrade,
) (SwapRequest, multichain.Listing, refusal, error) {
	untradable := refusal{code: errs.CodeAssetUntradable}
	winner, err := h.winner(ctx, cmd)
	switch code := errs.CodeOf(err); {
	case code == errs.CodeAssetNotFound || code == errs.CodeAssetUntradable:
		return SwapRequest{}, multichain.Listing{}, untradable, nil
	case pinRefused(code):
		return SwapRequest{}, multichain.Listing{}, refusal{code: code}, nil
	case err != nil:
		return SwapRequest{}, multichain.Listing{}, refusal{}, err
	}
	req, refused, err := h.tradable(ctx, cmd, winner)
	if refused.refused() || err != nil {
		return SwapRequest{}, multichain.Listing{}, refused, err
	}
	for _, next := range h.checks(cmd, winner, &req) {
		if refused, err := next(ctx, cmd, &req); refused.refused() || err != nil {
			return SwapRequest{}, multichain.Listing{}, refused, err
		}
	}
	return req, winner, refusal{}, nil
}

func pinRefused(code errs.Code) bool {
	return code == errs.CodePinnedChainUnavailable || code == errs.CodeChainTradingOff
}

func (h *ExecuteTradeHandler) checks(cmd ExecuteTrade, winner multichain.Listing, req *SwapRequest) []tradeCheck {
	token := chain.Mint{Address: chain.SolanaAddress(winner.TokenAddress), Decimals: winner.Decimals}
	switch {
	case winner.Network.IsEVM() && cmd.Action == domain.ActionSell:
		return []tradeCheck{h.held(evmAsset(winner.Network, winner.TokenAddress)), h.slippage, h.paused}
	case winner.Network.IsEVM():
		return []tradeCheck{h.routedFunds, h.slippage, h.paused}
	case winner.Network == chain.NetworkSolana && cmd.Action == domain.ActionSell:
		req.InMint = token
	case winner.Network == chain.NetworkSolana:
		req.OutMint = token
	}
	return []tradeCheck{h.funds, h.slippage, h.price, h.paused}
}

func (h *ExecuteTradeHandler) tradable(
	ctx context.Context, cmd ExecuteTrade, winner multichain.Listing,
) (SwapRequest, refusal, error) {
	untradable := refusal{code: errs.CodeAssetUntradable}
	asset, err := h.d.Ports.Catalog.AssetBySymbol(ctx, cmd.Symbol)
	routedRetry := winner.Network.IsEVM() &&
		string(cmd.Mint) == evmAsset(winner.Network, winner.TokenAddress)
	switch {
	case errs.CodeOf(err) == errs.CodeAssetNotFound:
		return SwapRequest{}, untradable, nil
	case err != nil:
		return SwapRequest{}, refusal{}, err
	case !asset.Tradable() || asset.Mint.Address() != cmd.Mint && !routedRetry:
		return SwapRequest{}, untradable, nil
	}
	wallet, err := h.d.Ports.Cabals.TreasuryWallet(ctx, cmd.CabalID)
	if err != nil {
		return SwapRequest{}, refusal{}, err
	}
	return h.request(cmd, asset, wallet), refusal{}, nil
}

func (h *ExecuteTradeHandler) held(asset string) tradeCheck {
	return func(ctx context.Context, cmd ExecuteTrade, req *SwapRequest) (refusal, error) {
		evm, err := evmListings(ctx, h.d.Ports.Positions, cmd.CabalID)
		if err != nil {
			return refusal{}, err
		}
		if have := evm[asset]; have < req.InAmount {
			return refusal{code: errs.CodeCabalSharesShort, have: have, need: req.InAmount}, nil
		}
		return refusal{}, nil
	}
}

func (h *ExecuteTradeHandler) routedFunds(ctx context.Context, cmd ExecuteTrade, req *SwapRequest) (refusal, error) {
	if cmd.Retry != nil {
		stranded, err := sqlc.New(h.d.Reads).StrandedCash(ctx, cmd.Retry.Of.UUID())
		req.ReusesStranded = stranded
		if err != nil || stranded {
			return refusal{}, err
		}
	}
	have, err := h.d.Ports.Balances.TokenBalance(ctx, req.TreasuryWallet.Address, req.InMint)
	switch {
	case err != nil:
		return refusal{}, err
	case have.Uint64() < req.InAmount:
		return refusal{code: errs.CodeInsufficientFunds, have: have.Uint64(), need: req.InAmount}, nil
	}
	return refusal{}, nil
}

func (h *ExecuteTradeHandler) request(cmd ExecuteTrade, asset market.Asset, w cabalport.TreasuryWallet) SwapRequest {
	token := chain.Mint{Address: cmd.Mint, Decimals: asset.Decimals}
	req := SwapRequest{
		Source: cmd.source(), CabalID: cmd.CabalID, Action: cmd.Action, Symbol: cmd.Symbol,
		TreasuryWallet: TreasuryWallet{PrivyWalletID: w.PrivyWalletID, Address: w.Address},
		InMint:         h.d.USDC, OutMint: token, InAmount: cmd.amount(), QuoteOutAmount: cmd.QuoteOutAmount,
		SourceBatchSize: 1,
		StillWanted: func(ctx context.Context) (bool, error) {
			return h.d.Ports.Proposals.Retryable(ctx, cmd.ProposalID)
		},
		Route: cmd.Route, Pin: cmd.Pin,
	}
	if cmd.Retry != nil && !cmd.unrouted {
		req.RouteOf = &cmd.Retry.Of
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
		cfg, err := h.d.Ports.Balances.MintConfig(ctx, req.OutMint.Address)
		if err != nil {
			return refusal{}, err
		}
		headroom := cfg.FeeHeadroom(req.InAmount)
		total, err := cmd.USDCMicros.Add(money.MicrosFromUint64(headroom))
		if err != nil {
			return refusal{}, err
		}
		need = total.Uint64()
	}
	if have.Uint64() < need {
		code := errs.CodeInsufficientFunds
		if cmd.Action == domain.ActionSell {
			code = errs.CodeCabalSharesShort
		}
		return refusal{code: code, have: have.Uint64(), need: need}, nil
	}
	return refusal{}, nil
}

func (h *ExecuteTradeHandler) slippage(ctx context.Context, cmd ExecuteTrade, req *SwapRequest) (refusal, error) {
	bps, err := h.slippageBps(ctx, cmd)
	req.SlippageBps = domain.SlippageOf(bps).Bps()
	return refusal{}, err
}

func (h *ExecuteTradeHandler) price(ctx context.Context, _ ExecuteTrade, req *SwapRequest) (refusal, error) {
	quote, err := h.d.Venue.Quote(ctx, QuoteSpec{InMint: req.InMint, OutMint: req.OutMint, InAmount: req.InAmount})
	if err != nil {
		return refusal{}, err
	}
	if !quote.Routable {
		return h.unrouted(ctx, req)
	}
	req.QuoteOutAmount = quote.OutAmount
	return refusal{}, nil
}

func (h *ExecuteTradeHandler) unrouted(ctx context.Context, req *SwapRequest) (refusal, error) {
	probe := uint64(1_000_000)
	if req.InMint.Address != h.d.USDC.Address {
		probe = min(req.InAmount, money.OneWhole(req.InMint.Decimals))
	}
	if req.InAmount <= probe {
		return refusal{code: errs.CodeNoRoute}, nil
	}
	quote, err := h.d.Venue.Quote(ctx, QuoteSpec{InMint: req.InMint, OutMint: req.OutMint, InAmount: probe})
	switch {
	case err != nil:
		return refusal{}, err
	case quote.Routable:
		return refusal{code: errs.CodeNoRoute}, nil
	}
	return refusal{code: errs.CodeAssetPaused}, nil
}

func (h *ExecuteTradeHandler) slippageBps(ctx context.Context, cmd ExecuteTrade) (int32, error) {
	if cmd.Retry != nil {
		return cmd.Retry.SlippageBps, nil
	}
	return h.d.Ports.Cabals.SlippageBps(ctx, cmd.CabalID)
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
	switch {
	case status == cabalport.StatusBanned:
		return refusal{code: errs.CodeCabalBanned}, nil
	case pause.Paused:
		return refusal{code: errs.CodeCabalPaused}, nil
	}
	return refusal{}, nil
}

func (h *ExecuteTradeHandler) block(ctx context.Context, d bus.Delivery, cmd ExecuteTrade, r refusal) error {
	blocked := false
	err := h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		inserted, err := d.RecordAs(ctx, tx, string(r.code))
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
