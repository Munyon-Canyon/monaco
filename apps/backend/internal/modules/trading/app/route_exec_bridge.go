package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type BridgeLeg struct {
	Ref      domain.StepRef
	WalletID string
	Spec     multichain.TransferSpec
	Fee      money.Micros
}

func stepKey(ref domain.StepRef) string {
	return "trade:" + ref.SwapID.String() + ":" + string(ref.Step)
}

type Bridge struct {
	Journal Journal
	Actions multichain.WalletActions
	Poll    Poll
}

func (b Bridge) Run(ctx context.Context, leg BridgeLeg) (domain.StepStatus, error) {
	step, err := b.Journal.Begin(ctx, leg.Ref, leg.Spec.AmountMicros)
	if err != nil {
		return "", err
	}
	id := multichain.ActionID(step.PrivyActionID)
	switch step.Status {
	case domain.StepPending:
		if id, err = b.submit(ctx, leg); err != nil {
			return "", err
		}
		if id == "" {
			return domain.StepFailed, nil
		}
	case domain.StepSucceeded, domain.StepFailed:
		return step.Status, nil
	case domain.StepSubmitted:
	}
	return b.await(ctx, leg, id)
}

func (b Bridge) submit(ctx context.Context, leg BridgeLeg) (multichain.ActionID, error) {
	id, err := b.Actions.Transfer(ctx, leg.WalletID, leg.Spec, stepKey(leg.Ref))
	code := errs.CodeOf(err)
	switch {
	case err != nil && errs.Retryable(code):
		return "", err
	case err != nil:
		_, err = b.Journal.Failed(ctx, leg.Ref, string(code))
		return "", err
	}
	faultpoint.Hit(ctx, faultpoint.AfterBroadcast)
	_, err = b.Journal.Submitted(ctx, leg.Ref, string(id))
	return id, err
}

func (b Bridge) await(ctx context.Context, leg BridgeLeg, id multichain.ActionID) (domain.StepStatus, error) {
	status := domain.StepSubmitted
	err := b.Poll.until(ctx, b.Journal.Clock, func() (bool, error) {
		action, err := b.Actions.Action(ctx, leg.WalletID, id)
		if err != nil {
			return false, err
		}
		faultpoint.Hit(ctx, faultpoint.AfterExecute)
		switch action.State {
		case multichain.ActionSucceeded:
			status = domain.StepSucceeded
			_, err = b.Journal.Succeeded(ctx, leg.Ref, leg.Fee)
		case multichain.ActionFailed, multichain.ActionRejected:
			status = domain.StepFailed
			_, err = b.Journal.Failed(ctx, leg.Ref, "privy_"+string(action.State))
		case multichain.ActionPending:
		}
		return status != domain.StepSubmitted, err
	})
	if err != nil {
		return "", err
	}
	return status, nil
}
