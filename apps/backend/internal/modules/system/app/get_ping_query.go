package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/system/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Ping struct {
	ID     uuid.UUID
	Note   string
	Echoed bool
}

func GetPing(ctx context.Context, q sqlc.DBTX, id uuid.UUID, user ids.UserID) (Ping, error) {
	const op = "system.GetPing"
	row, err := sqlc.New(q).GetPing(ctx, sqlc.GetPingParams{ID: id, UserID: user.UUID()})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Ping{}, errs.New(errs.CodeNotFound, op)
	case err != nil:
		return Ping{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	return Ping{ID: row.ID, Note: row.Note, Echoed: row.Echoed}, nil
}
