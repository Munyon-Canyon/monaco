package app

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Me struct {
	ID                  ids.UserID
	Handle              string
	DisplayName         string
	PhotoURL            string
	AuthState           domain.AuthState
	AccountStatus       domain.AccountStatus
	MemberWalletAddress chain.SolanaAddress
	PhoneLinked         bool
	XUsername           string
	HandleChangeableAt  *time.Time
	CreatedAt           time.Time
}

func GetMe(ctx context.Context, q sqlc.DBTX, id ids.UserID) (Me, error) {
	const op = "identity.GetMe"
	row, err := sqlc.New(q).GetMe(ctx, id.UUID())
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Me{}, errs.New(errs.CodeUserNotFound, op)
	case err != nil:
		return Me{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	me := Me{
		ID: id, Handle: row.Handle.String, DisplayName: cmp.Or(row.DisplayName, row.Handle.String),
		PhotoURL: row.PhotoUrl.String, AuthState: domain.AuthState(row.AuthState),
		AccountStatus: domain.AccountStatus(row.AccountStatus), MemberWalletAddress: chain.SolanaAddress(row.Address),
		PhoneLinked: row.PhoneLinked, XUsername: row.XUsername.String, CreatedAt: row.CreatedAt.UTC(),
	}
	if row.HandleChangedAt.Valid {
		at := row.HandleChangedAt.Time.UTC().Add(domain.HandleChangeInterval)
		me.HandleChangeableAt = &at
	}
	return me, nil
}
