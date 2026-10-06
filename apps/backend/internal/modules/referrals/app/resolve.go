package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const resolveOp = "referrals.Resolve"

type Resolved struct {
	UserID   ids.UserID
	CodeKind domain.CodeKind
	Code     domain.Code
}

type Resolver struct {
	Reads sqlc.DBTX
	Users identity.UserReader
}

func (r Resolver) Resolve(ctx context.Context, input string) (Resolved, error) {
	code := domain.NormalizeInput(input)
	if domain.IsRandomShape(code) {
		return r.random(ctx, domain.Code(code))
	}
	card, err := r.Users.UserByHandle(ctx, code)
	switch {
	case errs.CodeOf(err) == errs.CodeUserNotFound:
		return Resolved{}, unknown()
	case err != nil:
		return Resolved{}, err
	case card.FirstDepositAt == nil || !inGoodStanding(card):
		return Resolved{}, unknown()
	}
	return Resolved{UserID: card.ID, CodeKind: domain.CodeKindHandle, Code: domain.Code(code)}, nil
}

func (r Resolver) Referrer(ctx context.Context, resolved Resolved) (identity.UserCard, error) {
	cards, err := r.Users.UsersByID(ctx, []ids.UserID{resolved.UserID})
	if err != nil {
		return identity.UserCard{}, err
	}
	card, ok := cards[resolved.UserID]
	if !ok {
		return identity.UserCard{}, errs.New(errs.CodeInternal, resolveOp)
	}
	return card, nil
}

func (r Resolver) random(ctx context.Context, code domain.Code) (Resolved, error) {
	owner, err := sqlc.New(r.Reads).ReferralCodeOwner(ctx, string(code))
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Resolved{}, unknown()
	case err != nil:
		return Resolved{}, errs.Wrap(err, errs.CodeInternal, resolveOp)
	}
	id, err := ids.ParseUserID(owner.String())
	if err != nil {
		return Resolved{}, errs.Wrap(err, errs.CodeDecodeFailed, resolveOp)
	}
	cards, err := r.Users.UsersByID(ctx, []ids.UserID{id})
	if err != nil {
		return Resolved{}, err
	}
	if card, ok := cards[id]; !ok || !inGoodStanding(card) {
		return Resolved{}, unknown()
	}
	return Resolved{UserID: id, CodeKind: domain.CodeKindRandom, Code: code}, nil
}

func inGoodStanding(card identity.UserCard) bool {
	return card.AccountStatus == identity.AccountActive && !card.Deleted
}

func unknown() error { return errs.New(errs.CodeReferralCodeUnknown, resolveOp) }
