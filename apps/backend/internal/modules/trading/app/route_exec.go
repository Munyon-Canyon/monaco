package app

import (
	"context"
	"database/sql"
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
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
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
	EVMArrivals EVMArrivals
	Cash        EVMCash
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
	id, owned, locked, err := l.claim(ctx, req, insert)
	if err == nil && owned {
		faultpoint.Hit(ctx, faultpoint.AfterCreate)
		err = l.beating(ctx, heartbeat, func(ctx context.Context) error {
			wanted, err := l.wanted(ctx, req, id, locked)
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
	l := r.d.Layer
	holder := l.d.IDs.NewV7()
	leased, err := r.lease(ctx, id, holder)
	if err == nil && !leased {
		_, err = r.loadRow(ctx, id)
	}
	if err != nil || !leased {
		return err
	}
	defer r.unlease(ctx, id, holder)
	return l.beating(ctx, func() { _, _ = r.lease(ctx, id, holder) }, func(ctx context.Context) error {
		return r.drive(ctx, id)
	})
}

func (r *RouteExec) lease(ctx context.Context, id, holder uuid.UUID) (bool, error) {
	now := r.d.Layer.d.Clock.Now()
	n, err := sqlc.New(r.d.Layer.d.Reads).LeaseRouted(ctx, sqlc.LeaseRoutedParams{
		ID: id, Holder: holder, Until: now.Add(routeLeaseTTL), Now: now,
	})
	if err != nil {
		return false, errs.Wrap(err, errs.CodeInternal, routeExecOp, slog.String("swap_id", id.String()))
	}
	return n > 0, nil
}

func (r *RouteExec) unlease(ctx context.Context, id, holder uuid.UUID) {
	_, _ = sqlc.New(r.d.Layer.d.Reads).ReleaseRouted(context.WithoutCancel(ctx), sqlc.ReleaseRoutedParams{
		ID: id, Holder: holder,
	})
}

func (r *RouteExec) drive(ctx context.Context, id uuid.UUID) error {
	b, err := r.load(ctx, id)
	if err != nil || b.status != domain.StatusCreated && b.status != domain.StatusSubmitted {
		return err
	}
	if b.req.Action == domain.ActionSell && b.req.InMint.Address == b.req.OutMint.Address {
		return r.cashLeg(ctx, b)
	}
	if b.req.Action == domain.ActionSell {
		return r.swapOnce(ctx, b, money.Micros{}, b.sellLeg())
	}
	var requote swapQuote
	if b.status == domain.StatusCreated {
		var stop bool
		if b, requote, stop, err = r.bridgeAndQuote(ctx, b); err != nil || stop {
			return err
		}
	}
	return r.swapOnce(ctx, b, b.landed, b.swapLeg(b.landed, requote))
}

func (r *RouteExec) swapOnce(ctx context.Context, b routedBuy, landed money.Micros, leg EVMSwapLeg) error {
	held, err := r.lockTreasury(ctx, b)
	if err != nil || !held {
		return err
	}
	result, err := r.swap().Run(ctx, leg)
	return r.settle(ctx, b, landed, result, err)
}

func (r *RouteExec) lockTreasury(ctx context.Context, b routedBuy) (bool, error) {
	l := r.d.Layer
	held := false
	err := r.d.Poll.until(ctx, r.d.Journal.Clock, func() (bool, error) {
		now := r.d.Journal.Clock.Now()
		n, err := sqlc.New(l.d.Reads).LockEVMSwaps(ctx, sqlc.LockEVMSwapsParams{
			CabalID: b.req.CabalID.UUID(), SwapID: b.id, ExpiresAt: now.Add(evmTreasuryLockTTL), Now: now,
		})
		if err != nil {
			return false, errs.Wrap(err, errs.CodeInternal, routeExecOp, slog.String("swap_id", b.id.String()))
		}
		held = n > 0
		return held, nil
	})
	return held, err
}

func (r *RouteExec) bridge() Bridge {
	return Bridge{
		Journal: r.d.Journal, Actions: r.d.Actions, TopUp: r.d.TopUp, Poll: r.d.Poll, Unknown: r.parkUnknown,
	}
}

const (
	parkedBridgeOutcomeUnknown = "bridge_outcome_unknown"
	ParkedSwapReceiptMissing   = "swap_receipt_missing"
	swapReceiptWait            = 10 * time.Minute
	routeLeaseTTL              = 2 * time.Minute
	evmTreasuryLockTTL         = 15 * time.Minute
)

func (r *RouteExec) parkUnknown(ctx context.Context, leg BridgeLeg, _ multichain.ActionID) error {
	if leg.Ref.Step != domain.StepBridgeOut {
		return nil
	}
	return r.park(ctx, leg.Ref.SwapID.UUID(), parkedBridgeOutcomeUnknown)
}

func (r *RouteExec) park(ctx context.Context, id uuid.UUID, code string) error {
	l := r.d.Layer
	now := l.d.Clock.Now()
	return l.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		row, err := sqlc.New(tx.Queries()).ParkRouted(ctx, sqlc.ParkRoutedParams{At: now, Code: code, ID: id})
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		tx.AfterCommit(func(ctx context.Context) {
			observability.Alert(ctx, observability.TradingRoutedSwapParked,
				slog.String("swap_id", id.String()), slog.String("cabal_id", row.CabalID.String()),
				slog.String("code", code), slog.String("step", row.Step))
		})
		return err
	})
}

func (r *RouteExec) parkUnconfirmed(ctx context.Context, b routedBuy) error {
	step, started, err := r.d.Journal.Step(ctx, ids.SwapIDFrom(b.id), domain.StepSwap)
	if err != nil || !started || r.d.Layer.d.Clock.Now().Sub(step.UpdatedAt) < swapReceiptWait {
		return err
	}
	return r.park(ctx, b.id, ParkedSwapReceiptMissing)
}

func (r *RouteExec) swap() EVMSwap {
	return EVMSwap{Journal: r.d.Journal, Venue: r.d.Venue, Sender: r.d.Sender, Poll: r.d.Poll}
}

type swapQuote struct {
	floor uint64
	route multichain.RouteOptions
}

func (r *RouteExec) bridgeAndQuote(ctx context.Context, b routedBuy) (routedBuy, swapQuote, bool, error) {
	leg := b.bridgeOut()
	leg.Observe = func(ctx context.Context, _ multichain.ActionStatus) (money.Micros, domain.Arrival, error) {
		step, _, err := r.d.Journal.Step(ctx, ids.SwapIDFrom(b.id), domain.StepBridgeOut)
		if err == nil {
			step.Arrival, err = r.observeOut(ctx, b, step)
		}
		if err == nil {
			b, err = b.arrived(step)
		}
		return b.fee, step.Arrival, err
	}
	status, err := r.bridge().Run(ctx, leg)
	switch {
	case err != nil:
		return b, swapQuote{}, true, err
	case status == domain.StepFailed:
		return b, swapQuote{}, true, r.finish(ctx, b, failed(b.req, b.id, domain.FailureNeverSubmitted, ""), "")
	case status != domain.StepSucceeded:
		return b, swapQuote{}, true, nil
	}
	faultpoint.Hit(ctx, faultpoint.AfterBridgeLanded)
	q, stop, err := r.requote(ctx, b)
	return b, q, stop, err
}

func (r *RouteExec) requote(ctx context.Context, b routedBuy) (swapQuote, bool, error) {
	quote, err := r.d.Quoter.Quote(ctx, b.listing, multichain.SideBuy, b.landed)
	if err != nil && errs.Retryable(errs.CodeOf(err)) {
		return swapQuote{}, true, err
	}
	out := uint64(0)
	if err == nil && quote.GrossOutput != nil && quote.GrossOutput.IsInt64() {
		out = quote.GrossOutput.Uint64()
	}
	floor := domain.SlippageOf(b.slippage).MinOut(out)
	strayed, siblingErr := false, error(nil)
	if floor != 0 {
		strayed, siblingErr = r.strayedFromSibling(ctx, b, quote)
	}
	if siblingErr != nil {
		return swapQuote{}, true, siblingErr
	}
	if floor == 0 || strayed {
		return swapQuote{}, true, r.finish(
			ctx, b, failed(b.req, b.id, domain.FailurePriceMoved, ""), string(domain.FailurePriceMoved))
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
		observability.Degraded(ctx, observability.TradingSwapRefused, slog.String("swap_id", b.id.String()),
			slog.String("step", string(domain.StepSwap)), slog.String("code", string(code)),
			slog.GroupAttrs("detail", errs.Detail(cause)...))
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
	return r.parkUnconfirmed(ctx, b)
}

func (r *RouteExec) finish(
	ctx context.Context, b routedBuy, s step, stepFailure string, then ...func(context.Context, db.Tx) error,
) error {
	l := r.d.Layer
	ref := b.ref(domain.StepSwap)
	return l.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var stepErr error
		if stepFailure == "" {
			_, stepErr = r.d.Journal.succeeded(ctx, tx, ref, money.Micros{}, domain.Arrival{})
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
	landed       money.Micros
	estimate     money.Micros
	slippage     int32
	solanaWallet string
	solana       chain.SolanaAddress
	sibling      multichain.ListingID
	evm          multichain.EVMWallet
	cash         chain.EVMAddress
	token        chain.EVMAddress
}

func (r *RouteExec) load(ctx context.Context, id uuid.UUID) (routedBuy, error) {
	b, err := r.loadRow(ctx, id)
	if err != nil {
		return routedBuy{}, err
	}
	return r.landing(ctx, b)
}

func (r *RouteExec) loadRow(ctx context.Context, id uuid.UUID) (routedBuy, error) {
	row, err := sqlc.New(r.d.Layer.d.Reads).RoutedSwap(ctx, id)
	if err != nil {
		return routedBuy{}, errs.Wrap(err, errs.CodeInternal, routeExecOp, slog.String("swap_id", id.String()))
	}
	var comparison multichain.Comparison
	jsonErr := json.Unmarshal(row.Comparison, &comparison)
	req, err := sweepRequest(row.CabalID, row.SourceKind, row.SourceID, row.Action, row.Symbol, row.InMint,
		row.InAmount, row.SourceBatchSize)
	sent, estimate := money.MicrosFromUint64(req.InAmount), bridgeFee(comparison)
	landed, landedErr := sent.Sub(estimate)
	if req.Action == domain.ActionSell {
		landedErr = nil
	}
	if err := errors.Join(jsonErr, err, landedErr); err != nil {
		return routedBuy{}, errs.Wrap(err, errs.CodeDecodeFailed, routeExecOp, slog.String("swap_id", id.String()))
	}
	req.OutMint = chain.Mint{Address: chain.SolanaAddress(row.OutMint)}
	b := routedBuy{
		id: id, req: req, status: domain.Status(row.Status), sent: sent,
		fee: estimate, landed: landed, estimate: estimate, slippage: row.SlippageBps,
		sibling: solanaSibling(comparison),
	}
	return r.resolve(ctx, b, comparison.Winner)
}

func (r *RouteExec) landing(ctx context.Context, b routedBuy) (routedBuy, error) {
	swap := ids.SwapIDFrom(b.id)
	step, started, err := r.d.Journal.Step(ctx, swap, domain.StepBridgeOut)
	if err != nil || !started || step.Status != domain.StepSucceeded {
		return b, err
	}
	if !step.Arrival.Observed() {
		step.Arrival, err = r.observeOut(ctx, b, step)
		if err == nil {
			step, err = r.d.Journal.Arrived(ctx, b.ref(domain.StepBridgeOut), step.Arrival)
		}
	}
	if err == nil {
		b, err = b.arrived(step)
	}
	return b, err
}

func (b routedBuy) arrived(step domain.TradeStep) (routedBuy, error) {
	fee, err := step.Amount.Sub(step.Arrival.Amount)
	b.fee, b.landed = fee, step.Arrival.Amount
	return b, err
}

func (r *RouteExec) observeOut(ctx context.Context, b routedBuy, step domain.TradeStep) (domain.Arrival, error) {
	var arrival domain.Arrival
	found := false
	exclude, err := r.d.Journal.ArrivalTxs(ctx, step.Network)
	if err == nil {
		arrival, found, err = r.d.EVMArrivals.Inbound(ctx, step.Network, b.cash, b.evm.Address, step.SentAt,
			arrivalFloor(step.Amount, b.estimate), step.Amount, exclude)
	}
	if err == nil && !found {
		err = errs.New(errs.CodeUpstreamUnavailable, "trading.RouteExec.observeOut",
			slog.String("swap_id", b.id.String()), slog.String("network", string(step.Network)),
			slog.String("reason", "bridge_out_not_arrived"))
	}
	if err != nil {
		return domain.Arrival{}, err
	}
	faultpoint.Hit(ctx, faultpoint.AfterArrivalObserved)
	return arrival, nil
}

func (r *RouteExec) resolve(ctx context.Context, b routedBuy, winner multichain.ListingID) (routedBuy, error) {
	listing, listingErr := r.d.Listings.Listing(ctx, winner)
	if listingErr == nil && b.req.InMint.Address == b.req.OutMint.Address {
		cash := listing.Network.Cash()
		listing.TokenAddress, listing.Decimals, listing.Symbol = cash.Address, cash.Decimals, cash.Symbol
	}
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
				TxSignature: chain.Signature(fill.Hash), ConfirmedAt: at, TokenDecimals: b.listing.Decimals,
			}
		},
	}
}
