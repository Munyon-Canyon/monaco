package adapters

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type FundTransferView struct {
	Status       domain.FundStatus
	AmountMicros string
	ShareUnits   string
	FailCode     string
}

func GetFundTransfer(ctx context.Context, reads sqlc.DBTX, caller ids.UserID, id uuid.UUID) (FundTransferView, error) {
	const op = "treasury.GetFundTransfer"
	row, err := sqlc.New(reads).GetFundTransfer(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.UserID != caller.UUID()) {
		return FundTransferView{}, errs.New(errs.CodeNotFound, op, slog.String("transfer_id", id.String()))
	}
	if err != nil {
		return FundTransferView{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	view := FundTransferView{
		Status:       domain.FundStatus(row.Status),
		AmountMicros: row.AmountMicros,
		FailCode:     row.FailCode,
	}
	if view.Status == domain.FundSettled {
		view.ShareUnits = row.ShareUnits
	}
	return view, nil
}
