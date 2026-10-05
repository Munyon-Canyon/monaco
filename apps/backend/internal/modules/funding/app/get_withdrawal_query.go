package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func GetWithdrawal(ctx context.Context, q sqlc.DBTX, id uuid.UUID, user ids.UserID) (sqlc.GetWithdrawalRow, error) {
	const op = "funding.GetWithdrawal"
	row, err := sqlc.New(q).GetWithdrawal(ctx, sqlc.GetWithdrawalParams{ID: id, UserID: user.UUID()})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return sqlc.GetWithdrawalRow{}, errs.New(errs.CodeNotFound, op)
	case err != nil:
		return sqlc.GetWithdrawalRow{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	return row, nil
}
