package app

import (
	"context"
	"log/slog"
	"slices"
	"strconv"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type BridgeLeg struct {
	Ref      domain.StepRef
	WalletID string
	Source   chain.SolanaAddress
	Spec     multichain.TransferSpec
	Fee      money.Micros
	Observe  func(context.Context, multichain.ActionStatus) (money.Micros, domain.Arrival, error)
}

func stepKey(ref domain.StepRef) string {
	return "trade:" + ref.SwapID.String() + ":" + string(ref.Step)
}

func attemptKey(ref domain.StepRef, attempt int) string {
	if attempt > 1 {
		return stepKey(ref) + ":" + strconv.Itoa(attempt)
	}
	return stepKey(ref)
}

type Bridge struct {
	Journal Journal
	Actions multichain.WalletActions
	TopUp   SOLTopUp
	Poll    Poll
	Unknown func(context.Context, BridgeLeg, multichain.ActionID) error
}

func actionBroadcast(action multichain.ActionStatus) bool {
	return slices.ContainsFunc(action.Steps, func(s multichain.ActionStep) bool {
		return s.TxHash != "" || s.State != multichain.StepFailed || s.Network == ""
	})
}

func (b Bridge) outcomeUnknown(ctx context.Context, leg BridgeLeg, id multichain.ActionID) error {
	observability.Alert(ctx, observability.TradingBridgeOutcomeUnknown,
		slog.String("swap_id", leg.Ref.SwapID.String()), slog.String("cabal_id", leg.Ref.CabalID.String()),
		slog.String("step", string(leg.Ref.Step)), slog.String("network", string(leg.Ref.Network)),
		slog.String("action_id", string(id)))
	return b.Unknown(ctx, leg, id)
}

func (b Bridge) Run(ctx context.Context, leg BridgeLeg) (domain.StepStatus, error) {
	step, err := b.Journal.Begin(ctx, leg.Ref, leg.Spec.AmountMicros)
	if err != nil {
		return "", err
	}
	id := multichain.ActionID(step.PrivyActionID)
	switch step.Status {
	case domain.StepPending:
		var state multichain.ActionState
		var prior multichain.ActionID
		if prior, state, err = b.landedLate(ctx, leg, step); err != nil {
			return "", err
		}
		if state == multichain.ActionPending {
			return domain.StepPending, nil
		}
		if state == multichain.ActionSucceeded {
			id = prior
			break
		}
		var settled domain.StepStatus
		if id, settled, err = b.submit(ctx, leg, step.Attempt); err != nil || settled != "" {
			return settled, err
		}
	case domain.StepSucceeded, domain.StepFailed:
		return step.Status, nil
	case domain.StepSubmitted:
	}
	return b.await(ctx, leg, id)
}

func (b Bridge) landedLate(
	ctx context.Context, leg BridgeLeg, step domain.TradeStep,
) (multichain.ActionID, multichain.ActionState, error) {
	state := multichain.ActionFailed
	for _, prior := range step.PriorPrivyActionIDs {
		action, err := b.Actions.Action(ctx, leg.WalletID, multichain.ActionID(prior))
		switch {
		case err != nil:
			return "", "", err
		case action.State == multichain.ActionSucceeded:
			_, err = b.Journal.Submitted(ctx, leg.Ref, prior)
			return multichain.ActionID(prior), action.State, err
		case action.State == multichain.ActionPending:
			state = action.State
		case action.State == multichain.ActionFailed && actionBroadcast(action):
			state = multichain.ActionPending
			if err = b.outcomeUnknown(ctx, leg, multichain.ActionID(prior)); err != nil {
				return "", "", err
			}
		}
	}
	return "", state, nil
}

func (b Bridge) submit(
	ctx context.Context, leg BridgeLeg, attempt int,
) (multichain.ActionID, domain.StepStatus, error) {
	if leg.Spec.From == chain.NetworkSolana {
		if err := b.TopUp.TopUpSOL(ctx, leg.Source); err != nil {
			return "", "", err
		}
	}
	id, err := b.Actions.Transfer(ctx, leg.WalletID, leg.Spec, attemptKey(leg.Ref, attempt))
	code := errs.CodeOf(err)
	switch {
	case err != nil && errs.Retryable(code):
		return "", "", err
	case code == errs.CodeDecodeFailed:
		return "", domain.StepPending, b.outcomeUnknown(ctx, leg, "")
	case err != nil:
		_, err = b.Journal.Failed(ctx, leg.Ref, string(code))
		return "", domain.StepFailed, err
	}
	faultpoint.Hit(ctx, faultpoint.AfterBroadcast)
	_, err = b.Journal.Submitted(ctx, leg.Ref, string(id))
	return id, "", err
}

func (b Bridge) await(ctx context.Context, leg BridgeLeg, id multichain.ActionID) (domain.StepStatus, error) {
	status := domain.StepSubmitted
	err := b.Poll.until(ctx, b.Journal.Clock, func() (bool, error) {
		action, err := b.Actions.Action(ctx, leg.WalletID, id)
		if err != nil {
			return false, err
		}
		faultpoint.Hit(ctx, faultpoint.AfterExecute)
		var done bool
		status, done, err = b.settle(ctx, leg, id, action)
		return done, err
	})
	if err != nil {
		return "", err
	}
	return status, nil
}

func (b Bridge) settle(
	ctx context.Context, leg BridgeLeg, id multichain.ActionID, action multichain.ActionStatus,
) (domain.StepStatus, bool, error) {
	switch action.State {
	case multichain.ActionSucceeded:
		fee, arrival := leg.Fee, domain.Arrival{}
		if leg.Observe != nil {
			var err error
			if fee, arrival, err = leg.Observe(ctx, action); err != nil {
				return domain.StepSubmitted, false, err
			}
		}
		_, err := b.Journal.Succeeded(ctx, leg.Ref, fee, arrival)
		return domain.StepSucceeded, true, err
	case multichain.ActionFailed, multichain.ActionRejected:
		if action.State == multichain.ActionFailed && actionBroadcast(action) {
			return domain.StepSubmitted, true, b.outcomeUnknown(ctx, leg, id)
		}
		_, err := b.Journal.Failed(ctx, leg.Ref, "privy_"+string(action.State))
		return domain.StepFailed, true, err
	case multichain.ActionPending:
	}
	return domain.StepSubmitted, false, nil
}
