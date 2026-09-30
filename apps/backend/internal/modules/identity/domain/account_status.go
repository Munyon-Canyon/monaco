package domain

import (
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type AccountStatus string

const (
	AccountActive    AccountStatus = "active"
	AccountSuspended AccountStatus = "suspended"
	AccountBanned    AccountStatus = "banned"
	AccountDeleted   AccountStatus = "deleted"
)

type AccountEvent string

const (
	AccountSuspend   AccountEvent = "suspend"
	AccountReinstate AccountEvent = "reinstate"
	AccountBan       AccountEvent = "ban"
	AccountUnban     AccountEvent = "unban"
	AccountDelete    AccountEvent = "delete"
)

type accountStep struct {
	from AccountStatus
	ev   AccountEvent
}

func accountSteps() map[accountStep]AccountStatus {
	return map[accountStep]AccountStatus{
		{AccountActive, AccountSuspend}:      AccountSuspended,
		{AccountSuspended, AccountReinstate}: AccountActive,
		{AccountActive, AccountBan}:          AccountBanned,
		{AccountSuspended, AccountBan}:       AccountBanned,
		{AccountBanned, AccountUnban}:        AccountActive,
		{AccountActive, AccountDelete}:       AccountDeleted,
		{AccountSuspended, AccountDelete}:    AccountDeleted,
		{AccountBanned, AccountDelete}:       AccountDeleted,
	}
}

func NextAccountStatus(from AccountStatus, ev AccountEvent) (AccountStatus, error) {
	next, ok := accountSteps()[accountStep{from, ev}]
	if !ok {
		return from, errs.New(errs.CodeAccountStatusTransition, "identity.NextAccountStatus",
			slog.String("from", string(from)), slog.String("event", string(ev)))
	}
	return next, nil
}
