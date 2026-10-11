package app

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const routeExecOp = "trading.RouteExec"

type RouteExecDeps struct {
	Layer       *SwapLayer
	Journal     Journal
	Wallets     Wallets
	TreasuryEVM TreasuryEVM
	Listings    Listings
	Quoter      multichain.Quoter
	Actions     multichain.WalletActions
	Sender      EVMSender
	Venue       EVMVenue
	TopUp       SOLTopUp
	Arrivals    Arrivals
	Poll        Poll
}

type RouteExec struct {
	d RouteExecDeps
}

func NewRouteExec(d RouteExecDeps) *RouteExec { return &RouteExec{d: d} }

func evmAsset(network chain.Network, address string) string {
	return string(network) + ":" + strings.ToLower(address)
}

func (r *RouteExec) Trade(
	ctx context.Context,
	req SwapRequest,
	listing multichain.Listing,
	heartbeat func(),
) (SwapView, error) {
	wallet, err := r.d.TreasuryEVM.EnsureTreasuryEVMWallet(ctx, req.CabalID)
	if err != nil {
		return SwapView{}, err
	}
	req.TreasuryWallet = TreasuryWallet{Address: chain.SolanaAddress(wallet.Address.String())}
	cash := chain.Mint{
		Address:  chain.SolanaAddress(evmAsset(listing.Network, listing.Network.Cash().Address)),
		Decimals: 6,
	}
	token := chain.Mint{
		Address:  chain.SolanaAddress(evmAsset(listing.Network, listing.TokenAddress)),
		Decimals: listing.Decimals,
	}
	req.InMint, req.OutMint = cash, token
	if req.Action == domain.ActionSell {
		req.InMint, req.OutMint = token, cash
	}
	l := r.d.Layer
	insert, err := req.insertParams(l.d.IDs.NewV7(), l.d.Clock.Now())
	if err != nil {
		return SwapView{}, err
	}
	id, owned, err := l.claim(ctx, req, insert)
	if err == nil && owned {
		faultpoint.Hit(ctx, faultpoint.AfterCreate)
		err = l.beating(ctx, heartbeat, func(ctx context.Context) error {
			wanted, err := l.wanted(ctx, req, id)
			if err != nil || !wanted {
				return err
			}
			return r.resume(ctx, id)
		})
	}
	if err != nil {
		return SwapView{}, err
	}
	return NewQueries(l.d.Reads).Swap(ctx, ids.SwapIDFrom(id))
}

func (r *RouteExec) Resume(ctx context.Context, id ids.SwapID) error { return r.resume(ctx, id.UUID()) }

func (r *RouteExec) resume(ctx context.Context, id uuid.UUID) error {
	b, err := r.load(ctx, id)
	if err != nil || b.status != domain.StatusCreated && b.status != domain.StatusSubmitted {
		return err
	}
	if b.req.Action == domain.ActionSell {
		result, err := r.swap().Run(ctx, b.sellLeg())
		return r.settle(ctx, b, money.Micros{}, result, err)
	}
	landed, err := b.sent.Sub(b.fee)
	if err != nil {
		return err
	}
	var requote swapQuote
	if b.status == domain.StatusCreated {
		var stop bool
		if requote, stop, err = r.bridgeAndQuote(ctx, b, landed); err != nil || stop {
			return err
		}
	}
	result, err := r.swap().Run(ctx, b.swapLeg(landed, requote))
	return r.settle(ctx, b, landed, result, err)
}

func (r *RouteExec) bridge() Bridge {
	return Bridge{Journal: r.d.Journal, Actions: r.d.Actions, TopUp: r.d.TopUp, Poll: r.d.Poll}
}

func (r *RouteExec) swap() EVMSwap {
	return EVMSwap{Journal: r.d.Journal, Venue: r.d.Venue, Sender: r.d.Sender, Poll: r.d.Poll}
}

type swapQuote struct {
	floor uint64
	route multichain.RouteOptions
}

func (r *RouteExec) bridgeAndQuote(ctx context.Context, b routedBuy, landed money.Micros) (swapQuote, bool, error) {
	status, err := r.bridge().Run(ctx, b.bridgeOut())
	switch {
	case err != nil:
		return swapQuote{}, true, err
	case status == domain.StepFailed:
		return swapQuote{}, true, r.finish(ctx, b, failed(b.req, b.id, domain.FailureNeverSubmitted, ""), "")
	case status != domain.StepSucceeded:
		return swapQuote{}, true, nil
	}
	faultpoint.Hit(ctx, faultpoint.AfterBridgeLanded)
	quote, err := r.d.Quoter.Quote(ctx, b.listing, multichain.SideBuy, landed)
	if err != nil && errs.Retryable(errs.CodeOf(err)) {
		return swapQuote{}, true, err
	}
	out := uint64(0)
	if err == nil && quote.GrossOutput != nil && quote.GrossOutput.IsUint64() {
		out = quote.GrossOutput.Uint64()
	}
	floor := domain.SlippageOf(b.slippage).MinOut(out)
	if floor == 0 {
		return swapQuote{}, true, r.finish(ctx, b, failed(b.req, b.id, domain.FailurePriceMoved, ""), "")
	}
	faultpoint.Hit(ctx, faultpoint.AfterRequote)
	return swapQuote{floor: floor, route: quote.Options}, false, nil
}

func (r *RouteExec) settle(
	ctx context.Context,
	b routedBuy,
	landed money.Micros,
	result EVMSwapResult,
	cause error,
) error {
	code := errs.CodeOf(cause)
	switch {
	case cause != nil && code == errs.CodePriceMoved:
		return r.finish(ctx, b, failed(b.req, b.id, domain.FailurePriceMoved, ""), string(code))
	case cause != nil && (errs.Retryable(code) || result.Hash != ""):
		return cause
	case cause != nil:
		return r.finish(ctx, b, failed(b.req, b.id, domain.FailureNeverSubmitted, ""), string(code))
	}
	switch {
	case result.State == FillSucceeded && b.req.Action == domain.ActionSell:
		return r.sold(ctx, b, result)
	case result.State == FillSucceeded:
		return r.finish(ctx, b, b.confirmed(landed, result), "")
	case result.State == FillReverted:
		return r.finish(ctx, b, failed(b.req, b.id, domain.FailureSwapReverted, ""), string(domain.FailureSwapReverted))
	}
	return nil
}

func (r *RouteExec) finish(
	ctx context.Context, b routedBuy, s step, stepFailure string, then ...func(context.Context, db.Tx) error,
) error {
	l := r.d.Layer
	ref := b.ref(domain.StepSwap)
	return l.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var stepErr error
		if stepFailure == "" {
			_, stepErr = r.d.Journal.succeeded(ctx, tx, ref, money.Micros{})
		} else {
			_, stepErr = r.d.Journal.failed(ctx, tx, ref, stepFailure)
		}
		at := l.d.Clock.Now()
		n, err := s.apply(ctx, sqlc.New(tx.Queries()), at)
		if err = errors.Join(stepErr, err); err != nil || n == 0 {
			return err
		}
		tx.AfterCommit(func(ctx context.Context) { l.committed(ctx, b.req, b.id, s) })
		err = tx.Events.Append(ctx, s.event(at))
		for _, next := range then {
			err = errors.Join(err, next(ctx, tx))
		}
		return err
	})
}

type routedBuy struct {
	id           uuid.UUID
	req          SwapRequest
	status       domain.Status
	listing      multichain.Listing
	sent         money.Micros
	fee          money.Micros
	slippage     int32
	solanaWallet string
	solana       chain.SolanaAddress
	evm          multichain.EVMWallet
	cash         chain.EVMAddress
	token        chain.EVMAddress
}

func (r *RouteExec) load(ctx context.Context, id uuid.UUID) (routedBuy, error) {
	row, err := sqlc.New(r.d.Layer.d.Reads).RoutedSwap(ctx, id)
	if err != nil {
		return routedBuy{}, errs.Wrap(err, errs.CodeInternal, routeExecOp, slog.String("swap_id", id.String()))
	}
	var comparison multichain.Comparison
	jsonErr := json.Unmarshal(row.Comparison, &comparison)
	req, err := sweepRequest(row.CabalID, row.SourceKind, row.SourceID, row.Action, row.Symbol, row.InMint,
		row.InAmount, row.SourceBatchSize)
	if err := errors.Join(jsonErr, err); err != nil {
		return routedBuy{}, errs.Wrap(err, errs.CodeDecodeFailed, routeExecOp, slog.String("swap_id", id.String()))
	}
	req.OutMint = chain.Mint{Address: chain.SolanaAddress(row.OutMint)}
	b := routedBuy{
		id: id, req: req, status: domain.Status(row.Status), sent: money.MicrosFromUint64(req.InAmount),
		fee: bridgeFee(comparison), slippage: row.SlippageBps,
	}
	return r.resolve(ctx, b, comparison.Winner)
}

func (r *RouteExec) resolve(ctx context.Context, b routedBuy, winner multichain.ListingID) (routedBuy, error) {
	listing, listingErr := r.d.Listings.Listing(ctx, winner)
	solana, solanaErr := r.d.Wallets.TreasuryWallet(ctx, b.req.CabalID)
	evm, evmErr := r.d.TreasuryEVM.EnsureTreasuryEVMWallet(ctx, b.req.CabalID)
	cash, cashErr := chain.ParseEVMAddress(listing.Network.Cash().Address)
	token, tokenErr := chain.ParseEVMAddress(listing.TokenAddress)
	if err := errors.Join(listingErr, solanaErr, evmErr, cashErr, tokenErr); err != nil {
		return routedBuy{}, err
	}
	b.listing, b.solanaWallet, b.solana, b.evm = listing, solana.PrivyWalletID, solana.Address, evm
	b.cash, b.token = cash, token
	b.req.OutMint.Decimals = listing.Decimals
	return b, nil
}

func bridgeFee(c multichain.Comparison) money.Micros {
	var fee money.Micros
	for _, q := range c.Ranked {
		if q.ListingID != c.Winner {
			continue
		}
		for _, cost := range q.Costs {
			if cost.Kind == multichain.CostBridge {
				fee = cost.Micros
			}
		}
	}
	return fee
}

func (b routedBuy) ref(kind domain.StepKind) domain.StepRef {
	return domain.StepRef{SwapID: ids.SwapIDFrom(b.id), CabalID: b.req.CabalID, Step: kind, Network: b.listing.Network}
}

func (b routedBuy) bridgeOut() BridgeLeg {
	return BridgeLeg{
		Ref: b.ref(domain.StepBridgeOut), WalletID: b.solanaWallet, Source: b.solana, Fee: b.fee,
		Spec: multichain.TransferSpec{
			From: chain.NetworkSolana, To: b.listing.Network, Destination: b.evm.Address.String(), AmountMicros: b.sent,
		},
	}
}

func (b routedBuy) swapLeg(landed money.Micros, q swapQuote) EVMSwapLeg {
	return EVMSwapLeg{
		Request: b.req, Ref: b.ref(domain.StepSwap), WalletID: b.evm.ID, Amount: landed,
		Spec: EVMSwapSpec{
			Network: b.listing.Network, Owner: b.evm.Address, TokenIn: b.cash, TokenOut: b.token,
			AmountIn: landed.Uint64(), MinOut: q.floor, SlippageBps: b.slippage, Route: q.route,
		},
	}
}

func (b routedBuy) confirmed(landed money.Micros, fill EVMSwapResult) step {
	req, id := b.req, b.id
	return step{
		id: id, from: domain.StatusSubmitted, status: domain.StatusConfirmed,
		apply: func(ctx context.Context, q *sqlc.Queries, at time.Time) (int64, error) {
			out, outOK := domain.Column(fill.Out)
			fee, feeOK := domain.Column(b.fee.Uint64())
			if !outOK || !feeOK {
				return 0, errs.New(errs.CodeDecodeFailed, routeExecOp, slog.Uint64("out_amount", fill.Out))
			}
			return q.FinishConfirmed(
				ctx,
				sqlc.FinishConfirmedParams{ID: id, OutAmount: out, FeeMicros: fee, ConfirmedAt: at},
			)
		},
		event: func(at time.Time) events.Event {
			return events.TradeConfirmed{
				V: 1, SwapID: id, CabalID: req.CabalID.UUID(), Source: req.tradeSource(),
				SourceBatchSize: req.SourceBatchSize, Action: string(req.Action), Symbol: req.Symbol,
				InMint: req.InMint.Address, InAmount: landed.Uint64(), OutMint: req.OutMint.Address,
				OutAmount: fill.Out, USDCMicros: b.sent, FeeMicros: b.fee,
				TxSignature: chain.Signature(fill.Hash), ConfirmedAt: at,
			}
		},
	}
}
