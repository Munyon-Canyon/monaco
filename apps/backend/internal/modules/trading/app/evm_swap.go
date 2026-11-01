package app

import (
	"context"
	"log/slog"
	"strconv"
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
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const evmSwapOp = "trading.EVMSwap"

type EVMSwapSpec struct {
	Network     chain.Network
	Owner       chain.EVMAddress
	TokenIn     chain.EVMAddress
	TokenOut    chain.EVMAddress
	AmountIn    uint64
	MinOut      uint64
	SlippageBps int32
	Route       multichain.RouteOptions
}

type EVMSwapPlan struct {
	Approvals  []multichain.EVMTransaction
	Swap       multichain.EVMTransaction
	OutAmount  uint64
	MinOut     uint64
	Candidates []EVMSwapCandidate
}

type EVMSwapCandidate struct {
	Route     multichain.RouteOptions
	Swap      multichain.EVMTransaction
	OutAmount uint64
}

type EVMFillState uint8

const (
	FillPending EVMFillState = iota + 1
	FillSucceeded
	FillReverted
)

type EVMFill struct {
	State EVMFillState
	Out   uint64
}

type EVMVenue interface {
	Build(ctx context.Context, spec EVMSwapSpec) (EVMSwapPlan, error)
	Choose(ctx context.Context, spec EVMSwapSpec, candidates []EVMSwapCandidate) (EVMSwapPlan, error)
	Fill(ctx context.Context, network chain.Network, txHash string, out, owner chain.EVMAddress) (EVMFill, error)
	Receipt(ctx context.Context, network chain.Network, txHash string) (EVMFillState, error)
}

type EVMSender interface {
	Send(ctx context.Context, walletID string, tx multichain.EVMTransaction, idemKey string) (string, error)
}

type UnwiredEVMVenue struct{}

func (UnwiredEVMVenue) Build(context.Context, EVMSwapSpec) (EVMSwapPlan, error) {
	return EVMSwapPlan{}, notImplemented("EVMVenue.Build", "#4970")
}

func (UnwiredEVMVenue) Choose(context.Context, EVMSwapSpec, []EVMSwapCandidate) (EVMSwapPlan, error) {
	return EVMSwapPlan{}, notImplemented("EVMVenue.Choose", "#4970")
}

func (UnwiredEVMVenue) Fill(
	context.Context,
	chain.Network,
	string,
	chain.EVMAddress,
	chain.EVMAddress,
) (EVMFill, error) {
	return EVMFill{}, notImplemented("EVMVenue.Fill", "#4970")
}

func (UnwiredEVMVenue) Receipt(context.Context, chain.Network, string) (EVMFillState, error) {
	return 0, notImplemented("EVMVenue.Receipt", "#4970")
}

type EVMSwapLeg struct {
	Request  SwapRequest
	Ref      domain.StepRef
	WalletID string
	Amount   money.Micros
	Spec     EVMSwapSpec
}

type EVMSwapResult struct {
	EVMFill
	Hash string
}

type EVMSwap struct {
	Journal Journal
	Venue   EVMVenue
	Sender  EVMSender
	Poll    Poll
}

func (s EVMSwap) Run(ctx context.Context, leg EVMSwapLeg) (EVMSwapResult, error) {
	step, err := s.Journal.Begin(ctx, leg.Ref, leg.Amount)
	if err != nil {
		return EVMSwapResult{}, err
	}
	hash := step.PrivyActionID
	if step.Status == domain.StepPending {
		if hash, err = s.send(ctx, leg); err != nil {
			return EVMSwapResult{Hash: hash}, err
		}
	}
	return s.await(ctx, leg, hash)
}

func (s EVMSwap) send(ctx context.Context, leg EVMSwapLeg) (string, error) {
	swap, err := s.swapTx(ctx, leg)
	if err != nil {
		return "", err
	}
	hash, err := s.Sender.Send(ctx, leg.WalletID, swap, stepKey(leg.Ref))
	if err != nil {
		return "", err
	}
	faultpoint.Hit(ctx, faultpoint.AfterBroadcast)
	if err := s.submit(ctx, leg, hash); err != nil {
		return hash, err
	}
	faultpoint.Hit(ctx, faultpoint.AfterSwapSubmitted)
	return hash, nil
}

func (s EVMSwap) swapTx(ctx context.Context, leg EVMSwapLeg) (multichain.EVMTransaction, error) {
	if stored, ok, err := s.Journal.SentTx(ctx, leg.Ref); err != nil || ok {
		return stored, err
	}
	plan, err := s.Venue.Build(ctx, leg.Spec)
	if err == nil && len(plan.Candidates) == 0 {
		err = floorMet(leg.Spec, plan)
	}
	if err != nil {
		return multichain.EVMTransaction{}, err
	}
	key := stepKey(leg.Ref)
	for i, approval := range plan.Approvals {
		sent, err := s.Sender.Send(ctx, leg.WalletID, approval, key+":approve:"+strconv.Itoa(i))
		if err == nil {
			err = s.approved(ctx, leg, sent)
		}
		if err != nil {
			return multichain.EVMTransaction{}, err
		}
	}
	if len(plan.Candidates) > 0 {
		if plan, err = s.chosen(ctx, leg.Spec, plan.Candidates); err != nil {
			return multichain.EVMTransaction{}, err
		}
	}
	return plan.Swap, s.Journal.RecordSentTx(ctx, leg.Ref, plan.Swap)
}

func (s EVMSwap) chosen(ctx context.Context, spec EVMSwapSpec, candidates []EVMSwapCandidate) (EVMSwapPlan, error) {
	plan, err := s.Venue.Choose(ctx, spec, candidates)
	if err == nil {
		err = floorMet(spec, plan)
	}
	return plan, err
}

func floorMet(spec EVMSwapSpec, plan EVMSwapPlan) error {
	floor := spec.MinOut
	if floor == 0 {
		floor = domain.SlippageOf(spec.SlippageBps).MinOut(plan.OutAmount)
	}
	if floor == 0 || plan.MinOut < floor {
		return errs.New(errs.CodePriceMoved, evmSwapOp,
			slog.Uint64("plan_min_out", plan.MinOut), slog.Uint64("floor", floor))
	}
	return nil
}

func (s EVMSwap) approved(ctx context.Context, leg EVMSwapLeg, hash string) error {
	state := FillPending
	err := s.Poll.until(ctx, s.Journal.Clock, func() (bool, error) {
		var err error
		state, err = s.Venue.Receipt(ctx, leg.Spec.Network, hash)
		return state != FillPending, err
	})
	attrs := []slog.Attr{slog.String("approval_hash", hash), slog.String("swap_id", leg.Ref.SwapID.String())}
	switch {
	case err != nil:
		return err
	case state == FillReverted:
		return errs.New(errs.CodeSwapFailed, evmSwapOp, append(attrs, slog.String("reason", "approval_reverted"))...)
	case state == FillPending:
		return errs.New(errs.CodeUpstreamUnavailable, evmSwapOp,
			append(attrs, slog.String("reason", "approval_unconfirmed"))...)
	}
	return nil
}

func (s EVMSwap) submit(ctx context.Context, leg EVMSwapLeg, hash string) error {
	return s.Journal.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		moved, err := s.Journal.submitted(ctx, tx, leg.Ref, hash, money.Micros{})
		if err != nil || !moved {
			return err
		}
		at := s.Journal.Clock.Now()
		next := submittedEVM(leg.Request, leg.Ref.SwapID.UUID(), hash)
		n, err := next.apply(ctx, sqlc.New(tx.Queries()), at)
		if err == nil && n == 0 {
			err = errs.New(errs.CodeVersionConflict, evmSwapOp, slog.String("swap_id", leg.Ref.SwapID.String()))
		}
		if err != nil {
			return err
		}
		return tx.Events.Append(ctx, next.event(at))
	})
}

func (s EVMSwap) await(ctx context.Context, leg EVMSwapLeg, hash string) (EVMSwapResult, error) {
	result := EVMSwapResult{Hash: hash, EVMFill: EVMFill{State: FillPending}}
	err := s.Poll.until(ctx, s.Journal.Clock, func() (bool, error) {
		fill, err := s.Venue.Fill(ctx, leg.Spec.Network, hash, leg.Spec.TokenOut, leg.Spec.Owner)
		if err != nil {
			return false, err
		}
		faultpoint.Hit(ctx, faultpoint.AfterExecute)
		result.EVMFill = fill
		return fill.State != FillPending, nil
	})
	if err != nil {
		return EVMSwapResult{Hash: hash}, err
	}
	return result, nil
}

func submittedEVM(req SwapRequest, id uuid.UUID, hash string) step {
	return step{
		id: id, from: domain.StatusCreated, status: domain.StatusSubmitted,
		apply: func(ctx context.Context, q *sqlc.Queries, at time.Time) (int64, error) {
			return q.MarkSubmittedEVM(ctx, sqlc.MarkSubmittedEVMParams{ID: id, TxHash: hash, SubmittedAt: at})
		},
		event: func(time.Time) events.Event { return req.submittedEvent(id, chain.Signature(hash)) },
	}
}
