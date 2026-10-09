package app

import (
	"context"
	"log/slog"
	"math/big"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	DepositResidualInterval = time.Hour

	residualAlertStreak = 2
)

type WalletLedger interface {
	WalletLedgerMicros(
		ctx context.Context, user ids.UserID, mint chain.SolanaAddress,
	) (money.SignedMicros, int, error)
}

func wholeMicros(raw string) *big.Int {
	n, _ := new(big.Int).SetString(raw, 10)
	return n
}

func (b *watchBudget) timeUp(now time.Time) bool {
	return !b.stopAt.IsZero() && !now.Before(b.stopAt)
}

func (p *DepositWatch) reconcile(ctx context.Context, budget *watchBudget) (int, int, error) {
	rows, err := sqlc.New(p.reads).DepositWatchReconcileWallets(ctx, sqlc.DepositWatchReconcileWalletsParams{
		Now: p.clock.Now(), RowLimit: depositWatchDirtyBatch,
	})
	if err != nil {
		return 0, 0, errs.Wrap(err, errs.CodeOf(err), "funding.DepositWatch.reconcile")
	}
	updates := make([]sqlc.SetDepositWatchReconcileParams, 0, len(rows))
	var alerts []residualAlert
	for _, row := range rows {
		if budget.timeUp(p.clock.Now()) {
			break
		}
		update, alert, err := p.checkResidual(ctx, row)
		if err != nil {
			return 0, 0, err
		}
		updates = append(updates, update)
		if alert != nil {
			alerts = append(alerts, *alert)
		}
	}
	if err := p.saveReconcile(ctx, updates); err != nil {
		return 0, 0, err
	}
	for _, alert := range alerts {
		observability.Degraded(ctx, observability.FundingDepositResidual,
			slog.String("wallet_address", alert.wallet), slog.String("user_id", alert.user),
			slog.String("residual_micros", alert.residual), slog.String("observed_micros", alert.observed),
			slog.String("ledger_micros", alert.ledger))
	}
	return len(updates), 0, nil
}

func (p *DepositWatch) saveReconcile(ctx context.Context, updates []sqlc.SetDepositWatchReconcileParams) error {
	if len(updates) == 0 {
		return nil
	}
	err := p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		for _, update := range updates {
			if err := q.SetDepositWatchReconcile(ctx, update); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return errs.Wrap(err, errs.CodeOf(err), "funding.DepositWatch.saveReconcile")
	}
	return nil
}

type residualAlert struct {
	wallet, user, residual, observed, ledger string
}

func (p *DepositWatch) checkResidual(
	ctx context.Context, row sqlc.DepositWatchReconcileWalletsRow,
) (sqlc.SetDepositWatchReconcileParams, *residualAlert, error) {
	user := ids.UserIDFrom(row.UserID)
	settled, pending, err := p.ledger.WalletLedgerMicros(ctx, user, p.usdc)
	if err != nil {
		return sqlc.SetDepositWatchReconcileParams{}, nil, errs.Wrap(
			err, errs.CodeOf(err), "funding.DepositWatch.reconcile.ledger")
	}
	update := sqlc.SetDepositWatchReconcileParams{
		WalletAddress: row.WalletAddress, OpeningMicros: row.OpeningMicros,
		ReconcileDueAt: p.clock.Now().Add(DepositResidualInterval),
	}
	if pending > 0 || row.PendingCandidates || row.Dirty {
		return update, nil, nil
	}
	observed, ledger := wholeMicros(row.Observed), wholeMicros(settled.String())
	if row.OpeningMicros == "" {
		update.OpeningMicros = new(big.Int).Sub(observed, ledger).String()
		return update, nil, nil
	}
	residual := new(big.Int).Sub(observed, new(big.Int).Add(wholeMicros(row.OpeningMicros), ledger))
	if residual.Sign() == 0 {
		return update, nil, nil
	}
	update.ResidualStreak = row.ResidualStreak + 1
	if update.ResidualStreak < residualAlertStreak {
		return update, nil, nil
	}
	return update, &residualAlert{
		wallet: row.WalletAddress, user: user.String(), residual: residual.String(),
		observed: row.Observed, ledger: ledger.String(),
	}, nil
}

func ObserveDepositWatch(meter metric.Meter, reads sqlc.DBTX, c clock.Clock) {
	_, _ = meter.Int64ObservableGauge("monaco_funding_deposit_residual_wallets",
		metric.WithUnit("{wallet}"),
		metric.WithDescription("Member wallets with USDC the ledger does not explain on two checks in a row."),
		metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
			n, err := sqlc.New(reads).DepositWatchResidualWallets(ctx)
			if err != nil {
				return errs.Wrap(err, errs.CodeOf(err), "funding.DepositWatch.residualGauge")
			}
			o.Observe(n)
			return nil
		}))
	_, _ = meter.Int64ObservableGauge("monaco_funding_deposit_candidates_pending_oldest_seconds",
		metric.WithUnit("s"),
		metric.WithDescription("Age of the oldest deposit candidate still pending."),
		metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
			n, err := sqlc.New(reads).DepositCandidatesPendingOldestSeconds(ctx, c.Now())
			if err != nil {
				return errs.Wrap(err, errs.CodeOf(err), "funding.DepositWatch.pendingOldestGauge")
			}
			o.Observe(n)
			return nil
		}))
}
