package adapters

import (
	"context"
	"errors"
	"log/slog"
	"time"

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
	SliceBps     *int32
	FailCode     string
}

func GetFundTransfer(
	ctx context.Context, reads sqlc.DBTX, pot *Queries, caller ids.UserID, id uuid.UUID,
) (FundTransferView, error) {
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
		slice, err := pot.CabalPot(ctx, ids.CabalIDFrom(row.CabalID), caller, true)
		if err != nil && errs.CodeOf(err) != errs.CodePriceUnavailable {
			return FundTransferView{}, err
		}
		if err == nil {
			view.SliceBps = &slice.Me.SliceBps
		}
	}
	return view, nil
}

type FundHistory struct{ Reads sqlc.DBTX }

func (h FundHistory) FirstFundedAt(ctx context.Context, cabal ids.CabalID, user ids.UserID) (time.Time, bool, error) {
	at, err := sqlc.New(h.Reads).FirstFundedAt(ctx, sqlc.FirstFundedAtParams{
		CabalID: cabal.UUID(), UserID: user.UUID(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, errs.Wrap(err, errs.CodeInternal, "treasury.FirstFundedAt")
	}
	return at, true, nil
}
