package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func (r *RouteExec) BridgeBack(ctx context.Context, id ids.SwapID) error {
	b, err := r.load(ctx, id.UUID())
	if err != nil {
		return err
	}
	_, err = r.bridgeBack(ctx, b)
	return err
}

func (r *RouteExec) bridgeBack(ctx context.Context, b routedBuy) (domain.StepStatus, error) {
	amount, err := r.returning(ctx, b)
	if err != nil || amount.IsZero() {
		return "", err
	}
	leg := b.bridgeBack(amount)
	leg.Observe = func(ctx context.Context, action multichain.ActionStatus) (money.Micros, domain.Arrival, error) {
		arrived, err := r.arrival(ctx, b, action)
		if err != nil {
			return money.Micros{}, domain.Arrival{}, err
		}
		fee, err := amount.Sub(atMost(arrived, amount))
		return fee, domain.Arrival{}, err
	}
	return r.bridge().Run(ctx, leg)
}

func (r *RouteExec) arrival(ctx context.Context, b routedBuy, action multichain.ActionStatus) (money.Micros, error) {
	for _, s := range action.Steps {
		if s.Network != chain.NetworkSolana || s.TxHash == "" {
			continue
		}
		cash := chain.NetworkSolana.Cash()
		usdc := chain.Mint{Address: chain.SolanaAddress(cash.Address), Decimals: cash.Decimals}
		got, err := r.d.Arrivals.InboundAmount(ctx, chain.Signature(s.TxHash), b.solana, usdc)
		return money.MicrosFromUint64(got.Uint64()), err
	}
	return money.Micros{}, errs.New(errs.CodeUpstreamUnavailable, "trading.RouteExec.arrival",
		slog.String("swap_id", b.id.String()), slog.String("action_id", string(action.ID)))
}

func (r *RouteExec) observedBack(ctx context.Context, b routedBuy) (money.Micros, bool, error) {
	step, started, err := r.d.Journal.Step(ctx, ids.SwapIDFrom(b.id), domain.StepBridgeBack)
	if err != nil || !started || step.Status != domain.StepSucceeded {
		return money.Micros{}, false, err
	}
	action, err := r.d.Actions.Action(ctx, b.evm.ID, multichain.ActionID(step.PrivyActionID))
	if err != nil {
		return money.Micros{}, false, err
	}
	arrived, err := r.arrival(ctx, b, action)
	return atMost(arrived, step.Amount), err == nil, err
}

type StrandedCash struct {
	OnEVM money.Micros
	Back  money.Micros
}

func (r *RouteExec) Stranded(ctx context.Context, id ids.SwapID) (StrandedCash, error) {
	b, err := r.load(ctx, id.UUID())
	if err != nil {
		return StrandedCash{}, err
	}
	onEVM := b.landed
	back, backErr := onEVM.Sub(b.estimate)
	observed, landed, err := r.observedBack(ctx, b)
	if landed {
		back = observed
	}
	return StrandedCash{OnEVM: onEVM, Back: back}, errors.Join(backErr, err)
}

func (r *RouteExec) ReturnStranded(ctx context.Context, id ids.SwapID) (money.Micros, bool, error) {
	var status domain.StepStatus
	b, err := r.load(ctx, id.UUID())
	if err == nil {
		status, err = r.bridgeBack(ctx, b)
	}
	switch {
	case err != nil:
		return money.Micros{}, false, err
	case status == domain.StepSucceeded:
		landed, _, err := r.observedBack(ctx, b)
		return landed, err == nil, err
	case status == domain.StepFailed || status == "":
		return money.Micros{}, false, errs.New(errs.CodeSwapFailed, "trading.RouteExec.ReturnStranded",
			slog.String("swap_id", id.String()), slog.String("reason", "stranded_cash_not_returned"))
	}
	return money.Micros{}, false, nil
}

func (r *RouteExec) returning(ctx context.Context, b routedBuy) (money.Micros, error) {
	step, started, err := r.d.Journal.Step(ctx, ids.SwapIDFrom(b.id), domain.StepBridgeBack)
	if err != nil || started {
		return step.Amount, err
	}
	landed := b.landed
	claimed := false
	err = r.d.Layer.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		err := q.LockStrandedCash(ctx, b.id)
		if err == nil {
			claimed, err = q.StrandedCash(ctx, b.id)
		}
		if err != nil || !claimed {
			return err
		}
		_, err = r.d.Journal.begin(ctx, tx, b.ref(domain.StepBridgeBack), landed)
		return err
	})
	if err != nil || !claimed {
		return money.Micros{}, err
	}
	return landed, nil
}

func (b routedBuy) bridgeBack(landed money.Micros) BridgeLeg {
	return BridgeLeg{
		Ref: b.ref(domain.StepBridgeBack), WalletID: b.evm.ID, Fee: b.estimate,
		Spec: multichain.TransferSpec{
			From: b.listing.Network, To: chain.NetworkSolana, Destination: string(b.solana), AmountMicros: landed,
		},
	}
}

func atMost(m, ceiling money.Micros) money.Micros {
	if m.Cmp(ceiling) > 0 {
		return ceiling
	}
	return m
}
