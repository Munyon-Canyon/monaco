package authn

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

var _ auth.TokenVerifier = (*AdminVerifier)(nil)

type AdminVerifier struct {
	base auth.TokenVerifier
	pool *pgxpool.Pool
}

func NewAdminVerifier(base auth.TokenVerifier, pool *pgxpool.Pool) *AdminVerifier {
	return &AdminVerifier{base: base, pool: pool}
}

func (v *AdminVerifier) Verify(ctx context.Context, raw string) (auth.Actor, error) {
	actor, err := v.base.Verify(ctx, raw)
	if err != nil {
		return auth.Actor{}, err
	}
	if actor.Standing != auth.StandingActive {
		return auth.Actor{}, errs.New(errs.CodeAdminForbidden, "admin.AdminVerifier.Verify")
	}
	id, err := ids.ParseUserID(actor.ID)
	if err != nil {
		return auth.Actor{}, errs.New(errs.CodeAdminForbidden, "admin.AdminVerifier.Verify")
	}
	var role string
	err = v.pool.QueryRow(ctx, `SELECT role FROM admins WHERE user_id = $1 AND revoked_at IS NULL`, id.UUID()).
		Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Actor{}, errs.New(errs.CodeAdminForbidden, "admin.AdminVerifier.Verify")
	}
	if err != nil {
		return auth.Actor{}, errs.Wrap(err, errs.CodeDBUnavailable, "admin.AdminVerifier.Verify")
	}
	return auth.Actor{Kind: auth.ActorAdmin, ID: id.String(), Role: role}, nil
}

func (v *AdminVerifier) VerifyServiceToken(ctx context.Context, raw string) (string, error) {
	const op = "admin.AdminVerifier.VerifyServiceToken"
	name, err := sqlc.New(v.pool).LiveServiceTokenName(ctx, domain.HashServiceToken(raw))
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errs.New(errs.CodeUnauthorized, op)
	}
	if err != nil {
		return "", errs.Wrap(err, errs.CodeDBUnavailable, op)
	}
	return name, nil
}
