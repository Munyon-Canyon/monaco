package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type CreditDeposit struct {
	ID            uuid.UUID
	UserID        ids.UserID
	WalletAddress chain.SolanaAddress
	TxSignature   chain.Signature
	Amount        money.Micros
	Slot          int64
	BlockTime     time.Time
	CreditedAt    time.Time
}

type CreditDepositHandler struct {
	uow   *db.UnitOfWork
	hints HintPublisher
}

type HintPublisher interface {
	PublishHint(context.Context, string, []byte)
}

func NewCreditDepositHandler(uow *db.UnitOfWork, hints HintPublisher) *CreditDepositHandler {
	return &CreditDepositHandler{uow: uow, hints: hints}
}

func (h *CreditDepositHandler) Handle(ctx context.Context, cmd CreditDeposit) (bool, error) {
	credited := false
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		credited, err = h.Apply(ctx, tx, cmd)
		return err
	})
	return credited, err
}

func (h *CreditDepositHandler) Apply(ctx context.Context, tx db.Tx, cmd CreditDeposit) (bool, error) {
	if cmd.Amount.IsZero() {
		return false, errs.New(errs.CodeInvalidInput, "funding.CreditDeposit.Apply")
	}
	inserted, err := sqlc.New(tx.Queries()).InsertDeposit(ctx, sqlc.InsertDepositParams{
		ID: cmd.ID, UserID: cmd.UserID.UUID(), WalletAddress: string(cmd.WalletAddress),
		TxSignature: string(cmd.TxSignature), AmountMicros: cmd.Amount.String(), Slot: cmd.Slot,
		BlockTime: cmd.BlockTime, CreditedAt: cmd.CreditedAt,
	})
	if err != nil {
		return false, err
	}
	if inserted == 0 {
		observability.Debug(ctx, observability.FundingDepositDuplicate,
			slog.String("wallet_address", string(cmd.WalletAddress)))
		return false, nil
	}
	if err := tx.Events.Append(ctx, events.DepositCredited{
		V:             1,
		DepositID:     cmd.ID,
		UserID:        cmd.UserID.UUID(),
		WalletAddress: cmd.WalletAddress,
		AmountMicros:  cmd.Amount,
		TxSignature:   cmd.TxSignature,
		Slot:          cmd.Slot,
		BlockTime:     optionalTime(cmd.BlockTime),
	}); err != nil {
		return false, err
	}
	tx.AfterCommit(func(ctx context.Context) {
		h.hints.PublishHint(ctx, events.UserBalanceChangedHint(cmd.UserID), nil)
		observability.Info(
			ctx,
			observability.FundingDepositCredited,
			slog.String("wallet_address", string(cmd.WalletAddress)),
			slog.String("amount_micros", cmd.Amount.String()),
		)
	})
	return true, nil
}

func optionalTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}
