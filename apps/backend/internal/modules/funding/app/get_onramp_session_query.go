package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func GetOnrampSession(ctx context.Context, reads sqlc.DBTX, id uuid.UUID, user ids.UserID) (OnrampSession, error) {
	row, err := sqlc.New(reads).GetOnrampSession(ctx, sqlc.GetOnrampSessionParams{ID: id, UserID: user.UUID()})
	if errors.Is(err, sql.ErrNoRows) {
		return OnrampSession{}, errs.New(errs.CodeNotFound, "funding.GetOnrampSession")
	}
	if err != nil {
		return OnrampSession{}, errs.Wrap(err, errs.CodeInternal, "funding.GetOnrampSession")
	}
	suggested, err := suggestedAmount(row.SuggestedAmountMicros)
	if err != nil {
		return OnrampSession{}, err
	}
	out := OnrampSession{
		ID: row.ID, Status: domain.OnrampStatus(row.Status), SuggestedAmount: suggested, CreatedAt: row.CreatedAt,
	}
	if row.Completed {
		out.CompletedAt = &row.CompletedAt
	}
	return out, nil
}
