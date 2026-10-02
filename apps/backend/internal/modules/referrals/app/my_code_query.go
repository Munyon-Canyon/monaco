package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	myCodeOp   = "referrals.GetMyCode"
	linkPrefix = "https://monacolabs.xyz/r/"
)

type MyCode struct {
	Code           string
	Link           string
	HandleLink     *string
	HandleUnlocked bool
}

func (r Resolver) MyCode(ctx context.Context, user ids.UserID) (MyCode, error) {
	code, err := sqlc.New(r.Reads).ReferralCodeOfUser(ctx, user.UUID())
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return MyCode{}, errs.New(errs.CodeReferralCodePending, myCodeOp)
	case err != nil:
		return MyCode{}, errs.Wrap(err, errs.CodeInternal, myCodeOp)
	}
	cards, err := r.Users.UsersByID(ctx, []ids.UserID{user})
	if err != nil {
		return MyCode{}, err
	}
	card, ok := cards[user]
	if !ok {
		return MyCode{}, errs.New(errs.CodeUserNotFound, myCodeOp)
	}
	out := MyCode{Code: code, Link: linkPrefix + code, HandleUnlocked: card.FirstDepositAt != nil}
	if out.HandleUnlocked && card.Handle != "" {
		handleLink := linkPrefix + card.Handle
		out.HandleLink = &handleLink
	}
	return out, nil
}
