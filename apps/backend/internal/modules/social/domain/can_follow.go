package domain

import (
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type AccountStatus string

const (
	AccountUnknown   AccountStatus = ""
	AccountActive    AccountStatus = "active"
	AccountSuspended AccountStatus = "suspended"
	AccountBanned    AccountStatus = "banned"
	AccountDeleted   AccountStatus = "deleted"
)

func CanFollow(me, them ids.UserID, status AccountStatus) error {
	const op = "social.CanFollow"
	switch {
	case me == them:
		return errs.New(errs.CodeCannotFollowSelf, op)
	case status == AccountUnknown || status == AccountDeleted:
		return errs.New(errs.CodeUserNotFound, op, slog.String("status", string(status)))
	case status == AccountBanned:
		return errs.New(errs.CodeUserBanned, op)
	default:
		return nil
	}
}
